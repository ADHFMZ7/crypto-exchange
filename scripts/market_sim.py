#!/usr/bin/env python3
"""
Generate market activity against a running exchange.

The exchange has no external price feed: nothing moves until somebody crosses a
spread. A freshly reset database therefore shows an empty book, a blank tape and
a ticker reading "no trades yet", which makes the market-monitoring UI
impossible to look at and impossible to judge. This fills it in.

What it produces, deliberately:

  - depth on both sides, from makers who quote a two-sided market and requote
    as the price moves
  - a tape with both aggressor directions, so taker_side is not always "buy"
  - partial fills, from takers sized smaller than the quotes they hit
  - multi-level fills, from occasional sweeps that walk past the touch
  - cancellations, from makers pulling stale quotes
  - a price that trends rather than jitters, so a 24h change means something

Everything sent to the API is an integer count of minor units — satoshis for
BTC, cents for USD — and `price` is quote minor units per one WHOLE base unit,
matching the wire contract. There are no floats in any amount this script
sends; the random walk is integer arithmetic throughout.

Usage:

    # against a locally running API
    python3 scripts/market_sim.py

    # faster, reproducible, longer
    python3 scripts/market_sim.py --steps 400 --tick 0.05 --seed 7

Users are reused if they already exist, so it is safe to run repeatedly. Note
that funding is a deposit, not an assignment, so a second run tops the same
accounts up again. To start from nothing, reset the database (scripts/reset-db.sh)
and restart the API — the order book is in memory, so a reset without a restart
leaves the book holding orders the database no longer has.

It also doubles as a soak test for settlement and cancellation, which is where
the concurrency lives. After a run, these should all hold:

    -- nothing created or destroyed
    SELECT currency, sum(available + locked) FROM balances GROUP BY currency;

    -- every live order holds a lock, no terminal order does
    SELECT count(*) FROM orders
     WHERE (status IN ('filled','cancelled') AND locked_remaining <> 0)
        OR (status IN ('open','partially_filled') AND locked_remaining = 0);

    -- the per-order locks decompose the aggregate exactly (buy side / quote)
    SELECT (SELECT sum(locked_remaining) FROM orders
             WHERE side = 'buy' AND status IN ('open','partially_filled'))
         = (SELECT sum(locked) FROM balances WHERE currency = 'USD');
"""

import argparse
import json
import math
import random
import sys
import time
import urllib.error
import urllib.request

# ── the exchange's units ─────────────────────────────────────────────────────
#
# Read from the API rather than hardcoded: the registry is the authority on what
# a minor unit means, and a second copy here is a factor-of-10^n bug waiting to
# happen.

SATOSHI = 100_000_000  # one whole BTC, in base minor units
CENT = 100             # one whole USD, in quote minor units

PASSWORD = "simulation-password"


class ApiError(Exception):
    def __init__(self, status, detail):
        super().__init__(f"HTTP {status}: {detail}")
        self.status = status
        self.detail = detail


class Client:
    """Thin HTTP wrapper. Standard library only, so the script has no install step."""

    def __init__(self, base):
        self.base = base.rstrip("/")

    def request(self, method, path, token=None, body=None):
        data = None if body is None else json.dumps(body).encode()
        req = urllib.request.Request(self.base + path, data=data, method=method)
        if body is not None:
            req.add_header("Content-Type", "application/json")
        if token:
            req.add_header("Authorization", "Bearer " + token)

        try:
            with urllib.request.urlopen(req, timeout=10) as res:
                raw = res.read().decode().strip()
                return json.loads(raw) if raw else None
        except urllib.error.HTTPError as err:
            raise ApiError(err.code, err.read().decode().strip()) from None
        except urllib.error.URLError as err:
            raise SystemExit(
                f"Cannot reach the API at {self.base} ({err.reason}).\n"
                f"Start it with: cd src && go run ./cmd/main.go"
            ) from None

    # -- reference -----------------------------------------------------------
    def currencies(self):
        return self.request("GET", "/currencies")

    def markets(self):
        return self.request("GET", "/markets")

    # -- account -------------------------------------------------------------
    def signup(self, email, fullname):
        return self.request(
            "POST", "/users",
            body={"email": email, "fullname": fullname, "password": PASSWORD},
        )

    def login(self, email):
        return self.request(
            "POST", "/auth/login", body={"email": email, "password": PASSWORD}
        )["token"]

    def deposit(self, token, currency, minor):
        return self.request(
            "PATCH", "/wallets/me", token=token,
            body={"currency": currency, "amount": minor},
        )

    def wallet(self, token):
        return self.request("GET", "/wallets/me", token=token)

    # -- trading -------------------------------------------------------------
    def place(self, token, market, side, quantity, price):
        return self.request(
            "POST", "/orders", token=token,
            body={"market": market, "side": side, "quantity": quantity, "price": price},
        )

    def cancel(self, token, order_id):
        return self.request("DELETE", f"/orders/{order_id}", token=token)

    def orders(self, token):
        return self.request("GET", "/orders", token=token)["orders"]

    # -- market data ---------------------------------------------------------
    def tape(self, market, limit=25):
        return self.request("GET", f"/markets/{market}/trades?limit={limit}")["trades"]

    def book(self, market, levels=10):
        return self.request("GET", f"/orderbook/{market}?limit={levels}")

    def ticker(self, market):
        return self.request("GET", f"/markets/{market}/ticker")


def fmt(minor, exponent, width=0):
    """Render minor units as a decimal string. Integer arithmetic only."""
    negative = minor < 0
    minor = abs(minor)
    scale = 10 ** exponent
    whole, frac = divmod(minor, scale)
    text = f"{whole:,}" if exponent == 0 else f"{whole:,}.{frac:0{exponent}d}"
    if negative:
        text = "-" + text
    return text.rjust(width)


class Trader:
    """One simulated account, and whatever the strategy needs to remember."""

    def __init__(self, name, role, email):
        self.name = name
        self.role = role
        self.email = email
        self.token = None
        self.quotes = []        # order ids this trader has resting
        self.quoted_at = None   # the fair value those quotes were built around

    def __repr__(self):
        return f"<{self.name} {self.role}>"


class Simulation:
    def __init__(self, client, market, base_exp, quote_exp, rng, args):
        self.api = client
        self.market = market
        self.base_exp = base_exp
        self.quote_exp = quote_exp
        self.rng = rng
        self.args = args

        # Fair value, in quote minor units per whole base unit. Every price the
        # simulation derives is an integer offset from this.
        self.fair = args.price * 10 ** quote_exp
        self.momentum = 0
        # The tick scales with the price rather than being one whole quote unit.
        # A $1 tick is fine on a $50,000 asset and larger than the entire spread
        # on a $150 one. Rounding to a power of ten keeps prices readable, and
        # at $50,000 it lands back on exactly $1.
        reference = args.price * 10 ** quote_exp
        raw = max(1, reference // 50_000)
        self.tick = 10 ** int(math.log10(raw))

        self.lot = 10 ** max(0, base_exp - 3)      # size to a thousandth of a whole unit

        # Makers quote on a coarser grid than the tick. Quotes in a real book
        # cluster on round numbers, which is why levels hold several orders
        # rather than each maker inventing a price of its own — and the depth
        # panel's order count is only interesting if that happens.
        self.grid = self.tick * args.grid
        self.started = time.monotonic()

        self.traders = []
        self.seen_trades = set()
        self.placed = 0
        self.cancelled = 0
        self.rejected = 0

    # -- logging -------------------------------------------------------------
    def log(self, who, event, detail=""):
        elapsed = time.monotonic() - self.started
        print(f"[{elapsed:6.1f}s] {who:<10} {event:<9} {detail}", flush=True)

    def price_str(self, price):
        return fmt(price, self.quote_exp)

    def qty_str(self, qty):
        return fmt(qty, self.base_exp)

    # -- setup ---------------------------------------------------------------
    def enrol(self, name, role, base_funding, quote_funding):
        email = f"{name}@sim.local"
        trader = Trader(name, role, email)

        try:
            self.api.signup(email, name.replace("-", " ").title())
            created = True
        except ApiError:
            # Almost certainly a duplicate email from an earlier run. Logging in
            # tells us for sure, and is what we wanted either way.
            created = False

        trader.token = self.api.login(email)

        # Top up rather than set: deposits are signed deltas, and a returning
        # user already holds whatever the last run left them.
        if base_funding:
            self.api.deposit(trader.token, self.base, base_funding)
        if quote_funding:
            self.api.deposit(trader.token, self.quote, quote_funding)

        self.traders.append(trader)
        self.log(name, "joined" if created else "returned",
                 f"{self.qty_str(base_funding)} {self.base} + "
                 f"{self.price_str(quote_funding)} {self.quote} deposited")
        return trader

    def setup(self, symbol_base, symbol_quote, makers, takers, retail):
        self.base = symbol_base
        self.quote = symbol_quote

        for i in range(makers):
            self.enrol(f"mm-{i + 1}", "maker", 40 * SATOSHI, 3_000_000 * CENT)
        for i in range(takers):
            self.enrol(f"taker-{i + 1}", "taker", 20 * SATOSHI, 1_500_000 * CENT)
        for i in range(retail):
            self.enrol(f"retail-{i + 1}", "retail", 5 * SATOSHI, 400_000 * CENT)

        self.makers = [t for t in self.traders if t.role == "maker"]
        self.takers = [t for t in self.traders if t.role == "taker"]
        self.retail = [t for t in self.traders if t.role == "retail"]

    # -- price process -------------------------------------------------------
    def step_price(self):
        """
        An integer random walk with persistent drift.

        Pure noise gives a tape that goes nowhere, and a 24h change that hovers
        at zero. Redrawing the drift only occasionally produces runs — stretches
        where the price trends — which is what makes the ticker and the tape
        worth looking at.
        """
        if self.rng.random() < 0.06:
            reach = self.args.trend
            self.momentum = self.rng.randint(-reach, reach)

        self.fair += self.momentum + self.rng.randint(-self.args.vol, self.args.vol)

        # Keep it in a believable band; a walk left alone eventually wanders to
        # absurdity or through zero, and a non-positive price is rejected.
        floor = (self.args.price // 4) * 10 ** self.quote_exp
        ceiling = self.args.price * 4 * 10 ** self.quote_exp
        self.fair = max(floor, min(ceiling, self.fair))

    def round_tick(self, price):
        return max(self.tick, (price // self.tick) * self.tick)

    def round_grid(self, price):
        return max(self.grid, (price // self.grid) * self.grid)

    def size(self, low, high):
        """
        A size in base minor units, skewed small and rounded to a lot.

        Two draws and the smaller one: most orders are small, occasionally one
        is not. A uniform distribution makes every level of the book look
        identical, which no real book does.

        Rounding matters for how the output reads. Nobody quotes 0.14280798 BTC;
        sizes cluster on round numbers, and a book full of full-precision noise
        looks generated even when the behaviour behind it is sound.
        """
        a = self.rng.randint(low, high)
        b = self.rng.randint(low, high)
        return max(self.lot, (min(a, b) // self.lot) * self.lot)

    # -- actions -------------------------------------------------------------
    def place(self, trader, side, quantity, price, tag="order"):
        try:
            ack = self.api.place(trader.token, self.market, side, quantity, price)
        except ApiError as err:
            self.rejected += 1
            # Running out of one side is normal and self-correcting: the trader
            # keeps the other side quoted until a fill replenishes them.
            self.log(trader.name, "rejected",
                     f"{side} {self.qty_str(quantity)} @ {self.price_str(price)} — {err.detail}")
            return None

        self.placed += 1
        self.log(trader.name, tag,
                 f"{side:<4} {self.qty_str(quantity)} {self.base} @ {self.price_str(price)}")
        return ack["order_id"]

    def cancel(self, trader, order_id):
        try:
            self.api.cancel(trader.token, order_id)
            self.cancelled += 1
            return True
        except ApiError as err:
            # 409 means it filled before the cancellation arrived. That is the
            # race working as designed, not a failure.
            if err.status != 409:
                self.log(trader.name, "cancel!", f"#{order_id} — {err.detail}")
            return False

    def act_maker(self, maker):
        """
        Quote a two-sided market, and requote when the price has moved away.

        The requote is what produces cancellations and keeps depth churning near
        the touch instead of leaving one stale wall that everything trades
        against.
        """
        drifted = (
            maker.quoted_at is None
            or abs(self.fair - maker.quoted_at) > self.args.requote
        )
        if not drifted:
            return

        for order_id in maker.quotes:
            self.cancel(maker, order_id)
        if maker.quotes:
            self.log(maker.name, "requote", f"pulled {len(maker.quotes)} quotes")
        maker.quotes = []

        # Inventory skew: a maker long the base leans its quotes down to sell
        # it back, and short leans them up. This is why the simulated price
        # mean-reverts around holdings rather than being pushed only by noise.
        skew = 0
        try:
            balances = {b["currency"]: b for b in self.api.wallet(maker.token)["balances"]}
            held = balances.get(self.base, {})
            total = int(held.get("available", 0)) + int(held.get("locked", 0))
            imbalance = total - self.args.inventory * SATOSHI
            skew = -(imbalance // SATOSHI) * (self.args.spread // 4)
        except (ApiError, ValueError):
            pass

        centre = self.fair + skew
        half = self.rng.randint(self.args.spread // 2, self.args.spread)

        # A ladder, not a single quote. Real makers show size at several prices,
        # and it is what gives the book levels to walk: a taker that sweeps past
        # the touch needs somewhere to keep filling, and the depth chart needs
        # more than one rung to be worth drawing.
        for rung in range(self.args.ladder):
            step = half + rung * self.rng.randint(self.tick, max(self.tick, self.args.spread))

            bid = self.round_grid(centre - step)
            ask = self.round_grid(centre + step) + self.grid
            if ask <= bid:
                ask = bid + self.grid

            # Size grows with distance: the touch is where a maker least wants
            # to be filled, so it shows least there.
            span = self.args.quote_size * (rung + 1)

            for side, price in (("buy", bid), ("sell", ask)):
                quantity = self.size(span // 4, span)
                order_id = self.place(maker, side, quantity, price, tag="quote")
                if order_id is not None:
                    maker.quotes.append(order_id)

        maker.quoted_at = self.fair

    def act_taker(self, taker, aggression=1):
        """
        Cross the spread against whatever is resting.

        The limit is priced through the touch rather than at it, so the order
        keeps filling as it walks levels — which is what produces trades at
        several prices from one order, and the partial fills behind them.
        """
        book = self.api.book(self.market, levels=5)
        bids, asks = book["bids"], book["asks"]

        side = self.rng.choice(("buy", "sell"))
        # Only cross a side that has something on it.
        if side == "buy" and not asks:
            side = "sell"
        if side == "sell" and not bids:
            side = "buy"
        if (side == "buy" and not asks) or (side == "sell" and not bids):
            return  # empty book, nothing to hit

        through = self.args.spread * 2 * aggression
        if side == "buy":
            limit = self.round_tick(asks[0]["price"] + through)
        else:
            limit = self.round_tick(max(self.tick, bids[0]["price"] - through))

        span = self.args.take_size * aggression
        quantity = self.size(span // 4, span)
        tag = "sweep" if aggression > 1 else "take"
        self.place(taker, side, quantity, limit, tag=tag)

        # A large aggressive order moves the market. Without this the price
        # process ignores its own trades, and the tape drifts back through the
        # sweep as if it had never happened.
        if aggression > 1:
            impact = self.args.vol * aggression
            self.momentum += impact if side == "buy" else -impact

    def act_retail(self, trader):
        """Small orders, usually passive, occasionally crossing. Noise with a wallet."""
        side = self.rng.choice(("buy", "sell"))
        quantity = self.size(self.args.retail_size // 4, self.args.retail_size)

        if self.rng.random() < 0.3:
            # Impatient: cross whatever is there.
            self.act_taker(trader)
            return

        # Patient: rest away from the touch, where it may sit for a long time.
        away = self.rng.randint(self.args.spread, self.args.spread * 6)
        price = self.fair - away if side == "buy" else self.fair + away
        self.place(trader, side, quantity, self.round_tick(price), tag="rest")

    def drain_tape(self):
        """Print trades that have appeared since the last look."""
        try:
            trades = self.api.tape(self.market, limit=25)
        except ApiError:
            return

        for trade in reversed(trades):
            if trade["id"] in self.seen_trades:
                continue
            self.seen_trades.add(trade["id"])
            arrow = "▲" if trade["taker_side"] == "buy" else "▼"
            self.log("— tape —", "TRADE",
                     f"{arrow} {self.qty_str(trade['quantity'])} {self.base} "
                     f"@ {self.price_str(trade['price'])}  (taker {trade['taker_side']})")

    # -- main loop -----------------------------------------------------------
    def run(self, steps):
        for step in range(steps):
            self.step_price()

            roll = self.rng.random()
            if roll < 0.45:
                self.act_maker(self.rng.choice(self.makers))
            elif roll < 0.75:
                self.act_taker(self.rng.choice(self.takers))
            elif roll < 0.97:
                self.act_retail(self.rng.choice(self.retail))
            else:
                # A whale. Rare by design — this is the event that produces
                # multi-level fills and a visible mark on the tape.
                whale = self.rng.choice(self.takers)
                self.log(whale.name, "WHALE", "sweeping the book")
                self.act_taker(whale, aggression=self.rng.randint(3, 6))

            # Settlement runs behind the 202, so give it a moment before the
            # tape is read — otherwise every trade is reported one step late.
            time.sleep(self.args.tick)
            self.drain_tape()

    # -- summary -------------------------------------------------------------
    def summary(self):
        print("\n" + "─" * 78)
        print(f"  {self.market} after {self.placed} orders, "
              f"{self.cancelled} cancellations, {self.rejected} rejections")
        print("─" * 78)

        ticker = self.api.ticker(self.market)
        if ticker["has_traded"]:
            change = ticker["change"]
            pct = (change / ticker["open_price"] * 100) if ticker["open_price"] else 0
            print(f"\n  last {self.price_str(ticker['last_price'])}   "
                  f"open {self.price_str(ticker['open_price'])}   "
                  f"change {'+' if change >= 0 else ''}{self.price_str(change)} "
                  f"({pct:+.2f}%)")
            print(f"  high {self.price_str(ticker['high'])}   "
                  f"low  {self.price_str(ticker['low'])}   "
                  f"volume {self.qty_str(ticker['base_volume'])} {self.base}   "
                  f"trades {ticker['trade_count']}")
        else:
            print("\n  nothing traded — the book never crossed")

        book = self.api.book(self.market, levels=6)
        print("\n  book")
        for level in reversed(book["asks"]):
            print(f"    ask  {self.price_str(level['price']):>14}  "
                  f"{self.qty_str(level['quantity']):>14}  ({level['orders']})")
        if book["bids"] and book["asks"]:
            spread = book["asks"][0]["price"] - book["bids"][0]["price"]
            print(f"    {'— spread ' + self.price_str(spread) + ' —':^40}")
        for level in book["bids"]:
            print(f"    bid  {self.price_str(level['price']):>14}  "
                  f"{self.qty_str(level['quantity']):>14}  ({level['orders']})")

        print("\n  accounts")
        for trader in self.traders:
            balances = {b["currency"]: b for b in self.api.wallet(trader.token)["balances"]}
            orders = self.api.orders(trader.token)
            resting = sum(1 for o in orders if o["status"] in ("open", "partially_filled"))
            filled = sum(1 for o in orders if o["status"] == "filled")
            partial = sum(1 for o in orders if o["status"] == "partially_filled")

            def held(code, exponent):
                row = balances.get(code, {})
                total = int(row.get("available", 0)) + int(row.get("locked", 0))
                return fmt(total, exponent, 16)

            print(f"    {trader.name:<10} {trader.role:<7} "
                  f"{held(self.base, self.base_exp)} {self.base}  "
                  f"{held(self.quote, self.quote_exp)} {self.quote}   "
                  f"resting {resting:>3}  filled {filled:>3}  partial {partial:>3}")

        print()


def main():
    parser = argparse.ArgumentParser(
        description="Generate realistic market activity against a running exchange.",
        formatter_class=argparse.ArgumentDefaultsHelpFormatter,
    )
    parser.add_argument("--api", default="http://localhost:8080", help="base URL of the API")
    parser.add_argument("--market", default=None, help="symbol to trade (default: the first listed)")
    parser.add_argument("--steps", type=int, default=150, help="how many actions to take")
    parser.add_argument("--tick", type=float, default=0.25, help="seconds between actions")
    parser.add_argument("--seed", type=int, default=None, help="seed for a reproducible run")

    parser.add_argument("--makers", type=int, default=2, help="two-sided quoting accounts")
    parser.add_argument("--takers", type=int, default=3, help="spread-crossing accounts")
    parser.add_argument("--retail", type=int, default=3, help="small noise accounts")

    parser.add_argument("--price", type=int, default=50_000, help="starting price, in WHOLE quote units")
    parser.add_argument("--vol", type=int, default=None, help="per-step noise, in quote minor units")
    parser.add_argument("--trend", type=int, default=None, help="drift magnitude, in quote minor units")
    parser.add_argument("--spread", type=int, default=None, help="maker spread, in quote minor units")
    parser.add_argument("--requote", type=int, default=None, help="price move that triggers a requote")
    parser.add_argument("--ladder", type=int, default=3, help="price levels a maker quotes per side")
    parser.add_argument("--grid", type=int, default=25, help="maker quotes round to this many ticks")

    parser.add_argument("--quote-size", type=int, default=None, help="maker size, in base minor units")
    parser.add_argument("--take-size", type=int, default=None, help="taker size, in base minor units")
    parser.add_argument("--retail-size", type=int, default=None, help="retail size, in base minor units")
    parser.add_argument("--inventory", type=int, default=40, help="maker's target base holding, in WHOLE units")

    args = parser.parse_args()
    rng = random.Random(args.seed)

    api = Client(args.api)

    markets = api.markets()
    if not markets:
        raise SystemExit("The exchange lists no markets.")

    market = next(
        (m for m in markets if m["symbol"] == args.market), None
    ) if args.market else markets[0]
    if market is None:
        listed = ", ".join(m["symbol"] for m in markets)
        raise SystemExit(f"No market {args.market!r}. Listed: {listed}")

    exponents = {c["code"]: c["exponent"] for c in api.currencies()}

    # Order sizes default to a fraction of one WHOLE base unit, not to a fixed
    # number of satoshis. SOL holds nine decimals where BTC holds eight, so a
    # hardcoded 1e8 is half a bitcoin on one market and a twentieth of a Solana
    # on the next — which reads as an anaemic book rather than as a bug.
    whole = 10 ** exponents[market["base"]]
    for flag, fraction in (("quote_size", 2), ("take_size", 4), ("retail_size", 12)):
        if getattr(args, flag) is None:
            setattr(args, flag, whole // fraction)

    # Volatility and spread default to fractions of the starting price rather
    # than to absolute minor units. The BTC-era defaults were those same
    # fractions of $50,000; kept absolute they are a rounding error on an
    # expensive asset and a total collapse on a cheap one — a $40 step is
    # nothing to bitcoin and a quarter of a solana.
    reference = args.price * 10 ** exponents[market["quote"]]
    for flag, per_10k in (("vol", 8), ("trend", 5), ("spread", 24), ("requote", 18)):
        if getattr(args, flag) is None:
            setattr(args, flag, max(1, reference * per_10k // 10_000))

    sim = Simulation(
        api, market["symbol"],
        exponents[market["base"]], exponents[market["quote"]],
        rng, args,
    )

    print(f"Simulating {market['symbol']} on {args.api}"
          f"{f' (seed {args.seed})' if args.seed is not None else ''}\n")

    sim.setup(market["base"], market["quote"], args.makers, args.takers, args.retail)
    print()

    try:
        sim.run(args.steps)
    except KeyboardInterrupt:
        print("\ninterrupted — summarising what happened so far", file=sys.stderr)

    sim.summary()


if __name__ == "__main__":
    main()
