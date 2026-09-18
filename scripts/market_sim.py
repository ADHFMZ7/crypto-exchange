#!/usr/bin/env python3
"""
Generate market activity against a running exchange, on every market it lists,
and measure what the exchange did with it.

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

Every listed market runs, round-robin, off one shared roster of accounts — so a
trader holds BTC, ETH, SOL and USD at once, and the three matching workers are
exercised together rather than one at a time. Round-robin rather than a random
pick because coverage should not depend on the seed: with --steps 240 and three
markets, each one gets exactly eighty actions.

Everything sent to the API is an integer count of minor units — satoshis for
BTC, lamports for SOL, cents for USD — and `price` is quote minor units per one
WHOLE base unit, matching the wire contract. There are no floats in any amount
this script sends; the random walk is integer arithmetic throughout. Exponents
come from GET /currencies rather than from constants here, because the registry
is the authority on what a minor unit means and a second copy is a factor-of-10ⁿ
bug waiting to happen.

What it measures
----------------

Three different latencies, which are worth keeping apart:

  intake       the HTTP call itself — POST /orders locks funds, writes the
               order row and sends to the market's queue, then answers 202.
               Matching has not happened yet.
  settlement   202 to the execution being readable over REST. This is the
               matching worker, the ledger event, and the settlement worker's
               transaction. Measured directly by the probes, which cross a
               known resting order and poll the tape until it appears.
  throughput   how many orders a second the intake path sustains under
               concurrency, measured by a burst of passive orders that rest
               rather than match, and then cancelled so the book is left clean.

Everything else — fill rates, spreads, taker-side split, the conservation
check — is in the report at the end.

Usage:

    # against a locally running API
    python3 scripts/market_sim.py

    # faster, reproducible, longer, quieter
    python3 scripts/market_sim.py --steps 400 --tick 0.05 --seed 7 --quiet

    # one market only, and no measurement phases
    python3 scripts/market_sim.py --markets BTC-USD --probes 0 --burst 0

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

The first of those is checked from here as well — see the conservation section
of the report, which compares what the roster held before trading with what it
holds after. Trading moves money between accounts and never mints it, so the
two totals must agree exactly.
"""

import argparse
import concurrent.futures
import datetime
import json
import math
import random
import sys
import threading
import time
import urllib.error
import urllib.request

PASSWORD = "simulation-password"

# Starting prices in WHOLE quote units, by base currency. Only a plausible
# opening mark — the random walk takes over from here — but the right order of
# magnitude matters, because tick size, spread and order size are all derived
# from it. Anything unlisted starts at FALLBACK_PRICE.
DEFAULT_PRICES = {"BTC": 50_000, "ETH": 3_000, "SOL": 150}
FALLBACK_PRICE = 1_000


class ApiError(Exception):
    def __init__(self, status, detail):
        super().__init__(f"HTTP {status}: {detail}")
        self.status = status
        self.detail = detail


# ── measurement ──────────────────────────────────────────────────────────────


def percentile(values, p):
    """
    Nearest-rank percentile.

    No interpolation: with a few hundred samples, inventing a value between two
    measurements claims more precision than the sample has.
    """
    if not values:
        return 0.0
    ordered = sorted(values)
    rank = max(0, math.ceil(p / 100 * len(ordered)) - 1)
    return ordered[rank]


class Metrics:
    """
    Timings and counters for everything the script asked the exchange to do.

    Latency is measured around the HTTP call, so it is the number a client
    actually sees. For POST /orders that is the balance update, the order insert
    and a channel send — matching and settlement happen behind the 202 and are
    measured separately, by the settlement probes.

    Locked because the burst phase runs concurrently. Everything else is
    single-threaded, and pays one uncontended lock per request for the
    privilege of not having two accounting paths.
    """

    def __init__(self):
        self.lock = threading.Lock()
        self.latency = {}   # label -> [milliseconds]
        self.failures = {}  # label -> count
        self.counters = {}  # name  -> int
        self.started = time.monotonic()

    def record(self, label, millis, ok=True):
        with self.lock:
            self.latency.setdefault(label, []).append(millis)
            if not ok:
                self.failures[label] = self.failures.get(label, 0) + 1

    def count(self, name, n=1):
        with self.lock:
            self.counters[name] = self.counters.get(name, 0) + n

    def get(self, name):
        return self.counters.get(name, 0)

    def requests(self):
        return sum(len(samples) for samples in self.latency.values())

    def elapsed(self):
        return time.monotonic() - self.started


def parse_time(text):
    """
    RFC3339 to an aware datetime.

    Go marshals a trailing Z and Postgres sends a numeric offset, and
    fromisoformat before 3.11 accepts neither Z nor an arbitrary number of
    fractional digits. Normalising both here keeps the settlement-lag figure
    available on whatever Python is to hand rather than only on the newest.
    """
    if text.endswith("Z"):
        text = text[:-1] + "+00:00"

    if "." in text:
        head, rest = text.split(".", 1)
        digits = ""
        for ch in rest:
            if not ch.isdigit():
                break
            digits += ch
        text = f"{head}.{(digits + '000000')[:6]}{rest[len(digits):]}"

    return datetime.datetime.fromisoformat(text)


def now_utc():
    return datetime.datetime.now(datetime.timezone.utc)


# ── the API ──────────────────────────────────────────────────────────────────


class Client:
    """Thin HTTP wrapper. Standard library only, so the script has no install step."""

    def __init__(self, base, metrics=None):
        self.base = base.rstrip("/")
        self.metrics = metrics

    def request(self, method, path, token=None, body=None, label=None):
        data = None if body is None else json.dumps(body).encode()
        req = urllib.request.Request(self.base + path, data=data, method=method)
        if body is not None:
            req.add_header("Content-Type", "application/json")
        if token:
            req.add_header("Authorization", "Bearer " + token)

        started = time.perf_counter()
        ok = True
        try:
            with urllib.request.urlopen(req, timeout=10) as res:
                raw = res.read().decode().strip()
                return json.loads(raw) if raw else None
        except urllib.error.HTTPError as err:
            ok = False
            raise ApiError(err.code, err.read().decode().strip()) from None
        except urllib.error.URLError as err:
            ok = False
            raise SystemExit(
                f"Cannot reach the API at {self.base} ({err.reason}).\n"
                f"Start it with: cd src && go run ./cmd/main.go"
            ) from None
        finally:
            if self.metrics is not None and label is not None:
                self.metrics.record(label, (time.perf_counter() - started) * 1000, ok)

    # -- reference -----------------------------------------------------------
    def currencies(self):
        return self.request("GET", "/currencies", label="currencies")

    def markets(self):
        return self.request("GET", "/markets", label="markets")

    # -- account -------------------------------------------------------------
    def signup(self, email, fullname):
        return self.request(
            "POST", "/users",
            body={"email": email, "fullname": fullname, "password": PASSWORD},
            label="signup",
        )

    def login(self, email):
        return self.request(
            "POST", "/auth/login", body={"email": email, "password": PASSWORD},
            label="login",
        )["token"]

    def deposit(self, token, currency, minor):
        return self.request(
            "PATCH", "/wallets/me", token=token,
            body={"currency": currency, "amount": minor},
            label="deposit",
        )

    def wallet(self, token):
        return self.request("GET", "/wallets/me", token=token, label="wallet")

    # -- trading -------------------------------------------------------------
    def place(self, token, market, side, quantity, price):
        return self.request(
            "POST", "/orders", token=token,
            body={"market": market, "side": side, "quantity": quantity, "price": price},
            label="place",
        )

    def cancel(self, token, order_id):
        return self.request("DELETE", f"/orders/{order_id}", token=token, label="cancel")

    def orders(self, token):
        return self.request("GET", "/orders", token=token, label="orders")["orders"]

    # -- market data ---------------------------------------------------------
    def tape(self, market, limit=25):
        return self.request(
            "GET", f"/markets/{market}/trades?limit={limit}", label="tape"
        )["trades"]

    def book(self, market, levels=10):
        return self.request("GET", f"/orderbook/{market}?limit={levels}", label="book")

    def ticker(self, market):
        return self.request("GET", f"/markets/{market}/ticker", label="ticker")


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
        # Keyed by market: one account quotes several books at once, and a
        # maker's stale quotes on BTC-USD say nothing about its quotes on SOL.
        self.quotes = {}      # symbol -> [order ids resting]
        self.quoted_at = {}   # symbol -> the fair value those quotes were built around

    def __repr__(self):
        return f"<{self.name} {self.role}>"


class MarketSim:
    """
    One market's price process, sizing and strategy state.

    The accounts are not in here — they are shared across every market by the
    Exchange, so one maker quotes all three books out of one set of balances.
    What is per-market is everything derived from the price: tick, lot, grid,
    spread, and the random walk itself.
    """

    def __init__(self, exchange, market, price):
        self.ex = exchange
        self.api = exchange.api
        self.rng = exchange.rng
        self.args = exchange.args

        self.symbol = market["symbol"]
        self.base = market["base"]
        self.quote = market["quote"]
        self.base_exp = exchange.exponents[self.base]
        self.quote_exp = exchange.exponents[self.quote]

        self.whole = 10 ** self.base_exp  # one whole base unit, in minor units
        self.opening = price

        # Fair value, in quote minor units per whole base unit. Every price the
        # simulation derives is an integer offset from this.
        self.fair = price * 10 ** self.quote_exp
        self.momentum = 0

        # The tick scales with the price rather than being one whole quote unit.
        # A $1 tick is fine on a $50,000 asset and larger than the entire spread
        # on a $150 one. Rounding to a power of ten keeps prices readable, and
        # at $50,000 it lands back on exactly $1.
        reference = price * 10 ** self.quote_exp
        raw = max(1, reference // 50_000)
        self.tick = 10 ** int(math.log10(raw))

        self.lot = 10 ** max(0, self.base_exp - 3)  # size to a thousandth of a whole unit

        # Makers quote on a coarser grid than the tick. Quotes in a real book
        # cluster on round numbers, which is why levels hold several orders
        # rather than each maker inventing a price of its own — and the depth
        # panel's order count is only interesting if that happens.
        self.grid = self.tick * self.args.grid

        # Order sizes are a fraction of one WHOLE base unit, not a fixed number
        # of satoshis. SOL holds nine decimals where BTC holds eight, so a
        # hardcoded 1e8 is half a bitcoin on one market and a twentieth of a
        # solana on the next — which reads as an anaemic book rather than a bug.
        self.quote_size = self.args.quote_size or self.whole // 2
        self.take_size = self.args.take_size or self.whole // 4
        self.retail_size = self.args.retail_size or self.whole // 12

        # Volatility and spread are fractions of the starting price rather than
        # absolute minor units, for the same reason. A $40 step is nothing to
        # bitcoin and a quarter of a solana.
        self.vol = self.args.vol or max(1, reference * 8 // 10_000)
        self.trend = self.args.trend or max(1, reference * 5 // 10_000)
        self.spread = self.args.spread or max(1, reference * 24 // 10_000)
        self.requote = self.args.requote or max(1, reference * 18 // 10_000)

        # The newest trade id already on the tape when this run started.
        #
        # The database outlives the script: a second run against the same
        # database finds the first run's executions still on the tape, and
        # without a floor it would count them as its own — inflating volume and
        # reporting a settlement lag measured from a trade that happened
        # yesterday.
        self.floor_id = 0

        # What happened on this market, for the report.
        self.placed = 0
        self.cancelled = 0
        self.rejected = 0
        self.trades = 0
        self.base_volume = 0
        self.quote_volume = 0
        self.taker_buy = 0
        self.taker_sell = 0
        self.lags = []  # milliseconds from match to the trade being readable

    # -- logging -------------------------------------------------------------
    def log(self, who, event, detail=""):
        if self.args.quiet:
            return
        elapsed = time.monotonic() - self.ex.started
        print(f"[{elapsed:6.1f}s] {self.symbol:<8} {who:<10} {event:<9} {detail}",
              flush=True)

    def price_str(self, price):
        return fmt(price, self.quote_exp)

    def qty_str(self, qty):
        return fmt(qty, self.base_exp)

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
            self.momentum = self.rng.randint(-self.trend, self.trend)

        self.fair += self.momentum + self.rng.randint(-self.vol, self.vol)

        # Keep it in a believable band; a walk left alone eventually wanders to
        # absurdity or through zero, and a non-positive price is rejected.
        floor = (self.opening // 4) * 10 ** self.quote_exp
        ceiling = self.opening * 4 * 10 ** self.quote_exp
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
            ack = self.api.place(trader.token, self.symbol, side, quantity, price)
        except ApiError as err:
            self.rejected += 1
            self.ex.metrics.count("rejected")
            # Running out of one side is normal and self-correcting: the trader
            # keeps the other side quoted until a fill replenishes them.
            self.log(trader.name, "rejected",
                     f"{side} {self.qty_str(quantity)} @ {self.price_str(price)} — {err.detail}")
            return None

        self.placed += 1
        self.ex.metrics.count("placed")
        self.log(trader.name, tag,
                 f"{side:<4} {self.qty_str(quantity)} {self.base} @ {self.price_str(price)}")
        return ack["order_id"]

    def cancel(self, trader, order_id):
        try:
            self.api.cancel(trader.token, order_id)
            self.cancelled += 1
            self.ex.metrics.count("cancelled")
            return True
        except ApiError as err:
            # 409 means it filled before the cancellation arrived. That is the
            # race working as designed, not a failure.
            if err.status == 409:
                self.ex.metrics.count("cancel_lost_race")
            else:
                self.log(trader.name, "cancel!", f"#{order_id} — {err.detail}")
            return False

    def act_maker(self, maker):
        """
        Quote a two-sided market, and requote when the price has moved away.

        The requote is what produces cancellations and keeps depth churning near
        the touch instead of leaving one stale wall that everything trades
        against.
        """
        quoted_at = maker.quoted_at.get(self.symbol)
        if quoted_at is not None and abs(self.fair - quoted_at) <= self.requote:
            return

        resting = maker.quotes.get(self.symbol, [])
        for order_id in resting:
            self.cancel(maker, order_id)
        if resting:
            self.log(maker.name, "requote", f"pulled {len(resting)} quotes")
        maker.quotes[self.symbol] = []

        # Inventory skew: a maker long the base leans its quotes down to sell
        # it back, and short leans them up. This is why the simulated price
        # mean-reverts around holdings rather than being pushed only by noise.
        #
        # The imbalance is measured in WHOLE base units, so it has to divide by
        # this market's own scale — 10^8 on BTC, 10^9 on SOL. A shared constant
        # here would make the skew ten times too small on Solana.
        skew = 0
        try:
            balances = {b["currency"]: b for b in self.api.wallet(maker.token)["balances"]}
            held = balances.get(self.base, {})
            total = int(held.get("available", 0)) + int(held.get("locked", 0))
            imbalance = total - self.args.inventory * self.whole
            skew = -(imbalance // self.whole) * (self.spread // 4)
        except (ApiError, ValueError):
            pass

        centre = self.fair + skew
        half = self.rng.randint(self.spread // 2, self.spread)

        # A ladder, not a single quote. Real makers show size at several prices,
        # and it is what gives the book levels to walk: a taker that sweeps past
        # the touch needs somewhere to keep filling, and the depth chart needs
        # more than one rung to be worth drawing.
        for rung in range(self.args.ladder):
            step = half + rung * self.rng.randint(self.tick, max(self.tick, self.spread))

            bid = self.round_grid(centre - step)
            ask = self.round_grid(centre + step) + self.grid
            if ask <= bid:
                ask = bid + self.grid

            # Size grows with distance: the touch is where a maker least wants
            # to be filled, so it shows least there.
            span = self.quote_size * (rung + 1)

            for side, price in (("buy", bid), ("sell", ask)):
                quantity = self.size(span // 4, span)
                order_id = self.place(maker, side, quantity, price, tag="quote")
                if order_id is not None:
                    maker.quotes[self.symbol].append(order_id)

        maker.quoted_at[self.symbol] = self.fair

    def act_taker(self, taker, aggression=1):
        """
        Cross the spread against whatever is resting.

        The limit is priced through the touch rather than at it, so the order
        keeps filling as it walks levels — which is what produces trades at
        several prices from one order, and the partial fills behind them.
        """
        book = self.api.book(self.symbol, levels=5)
        bids, asks = book["bids"], book["asks"]

        side = self.rng.choice(("buy", "sell"))
        # Only cross a side that has something on it.
        if side == "buy" and not asks:
            side = "sell"
        if side == "sell" and not bids:
            side = "buy"
        if (side == "buy" and not asks) or (side == "sell" and not bids):
            return  # empty book, nothing to hit

        through = self.spread * 2 * aggression
        if side == "buy":
            limit = self.round_tick(asks[0]["price"] + through)
        else:
            limit = self.round_tick(max(self.tick, bids[0]["price"] - through))

        span = self.take_size * aggression
        quantity = self.size(span // 4, span)
        tag = "sweep" if aggression > 1 else "take"
        self.place(taker, side, quantity, limit, tag=tag)

        # A large aggressive order moves the market. Without this the price
        # process ignores its own trades, and the tape drifts back through the
        # sweep as if it had never happened.
        if aggression > 1:
            impact = self.vol * aggression
            self.momentum += impact if side == "buy" else -impact

    def act_retail(self, trader):
        """Small orders, usually passive, occasionally crossing. Noise with a wallet."""
        side = self.rng.choice(("buy", "sell"))
        quantity = self.size(self.retail_size // 4, self.retail_size)

        if self.rng.random() < 0.3:
            # Impatient: cross whatever is there.
            self.act_taker(trader)
            return

        # Patient: rest away from the touch, where it may sit for a long time.
        away = self.rng.randint(self.spread, self.spread * 6)
        price = self.fair - away if side == "buy" else self.fair + away
        self.place(trader, side, quantity, self.round_tick(price), tag="rest")

    def drain_tape(self):
        """
        Print trades that have appeared since the last look, and time them.

        The lag recorded here is from the engine's own execution timestamp to
        the moment this loop read the row, so it includes however long the
        script waited before asking. It is an upper bound on settlement, not a
        measurement of it — the probes in Exchange.probe_settlement are the
        figure to quote.
        """
        try:
            trades = self.api.tape(self.symbol, limit=25)
        except ApiError:
            return

        observed = now_utc()

        for trade in reversed(trades):
            if trade["id"] <= self.floor_id or trade["id"] in self.ex.seen_trades:
                continue
            self.ex.seen_trades.add(trade["id"])

            self.trades += 1
            self.base_volume += int(trade["quantity"])
            self.quote_volume += int(trade["quantity"]) * int(trade["price"]) // self.whole
            if trade["taker_side"] == "buy":
                self.taker_buy += 1
            else:
                self.taker_sell += 1

            try:
                self.lags.append(
                    (observed - parse_time(trade["executed_at"])).total_seconds() * 1000
                )
            except (KeyError, ValueError):
                pass

            arrow = "▲" if trade["taker_side"] == "buy" else "▼"
            self.log("— tape —", "TRADE",
                     f"{arrow} {self.qty_str(trade['quantity'])} {self.base} "
                     f"@ {self.price_str(trade['price'])}  (taker {trade['taker_side']})")


class Exchange:
    """
    The whole run: one roster of accounts trading every listed market.

    Accounts are shared rather than per-market because that is what the
    exchange's own model says — a balance belongs to a (user, currency), not to
    a market — and because sharing them is what puts concurrent settlement
    against the same balance rows, which is the part worth soak-testing.
    """

    def __init__(self, api, metrics, rng, args, markets, exponents, prices):
        self.api = api
        self.metrics = metrics
        self.rng = rng
        self.args = args
        self.exponents = exponents
        self.started = time.monotonic()
        self.seen_trades = set()

        self.sims = {}
        for market in markets:
            symbol = market["symbol"]
            self.sims[symbol] = MarketSim(self, market, prices[symbol])

        self.traders = []
        self.makers = []
        self.takers = []
        self.retail = []
        self.baseline = {}  # currency -> minor units held by the roster before trading

    # -- setup ---------------------------------------------------------------
    def enrol(self, name, role, funding):
        """funding maps a currency code to an amount in that currency's minor units."""
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
        for currency, amount in funding.items():
            if amount:
                self.api.deposit(trader.token, currency, amount)

        self.traders.append(trader)

        if not self.args.quiet:
            held = "  ".join(
                f"{fmt(amount, self.exponents[code])} {code}"
                for code, amount in funding.items() if amount
            )
            elapsed = time.monotonic() - self.started
            print(f"[{elapsed:6.1f}s] {'setup':<8} {name:<10} "
                  f"{'joined' if created else 'returned':<9} {held}", flush=True)

        return trader

    def setup(self):
        """
        Enrol the roster and fund it in every currency any listed market needs.

        A trader gets the quote currency once, not once per market: USD is USD
        whichever book it is spent on, and depositing per market would fund the
        same account three times over and make the conservation check meaningless.
        """
        bases = []
        quotes = []
        for sim in self.sims.values():
            if sim.base not in bases:
                bases.append(sim.base)
            if sim.quote not in quotes:
                quotes.append(sim.quote)

        # Base funding is in WHOLE units, so a maker starts flat against the
        # --inventory target on every market and its skew begins at zero.
        roles = (
            ("mm", "maker", self.args.makers, self.args.inventory, 3_000_000),
            ("taker", "taker", self.args.takers, max(1, self.args.inventory // 2), 1_500_000),
            ("retail", "retail", self.args.retail, max(1, self.args.inventory // 8), 400_000),
        )

        for prefix, role, count, base_whole, quote_whole in roles:
            for i in range(count):
                funding = {
                    code: base_whole * 10 ** self.exponents[code] for code in bases
                }
                for code in quotes:
                    funding[code] = quote_whole * 10 ** self.exponents[code]
                self.enrol(f"{prefix}-{i + 1}", role, funding)

        self.makers = [t for t in self.traders if t.role == "maker"]
        self.takers = [t for t in self.traders if t.role == "taker"]
        self.retail = [t for t in self.traders if t.role == "retail"]

        # Everything already on the tape belongs to an earlier run.
        for sim in self.sims.values():
            try:
                tape = sim.api.tape(sim.symbol, limit=1)
            except ApiError:
                continue
            if tape:
                sim.floor_id = tape[0]["id"]

        # Snapshot now, so the conservation check at the end compares like with
        # like. Anything the roster holds before the first order is the total it
        # must still hold after the last one: trading moves money between these
        # accounts and never creates it.
        self.baseline = self.holdings()

    def holdings(self):
        """Total held by the roster, per currency, available plus locked."""
        totals = {}
        for trader in self.traders:
            for row in self.api.wallet(trader.token)["balances"]:
                amount = int(row.get("available", 0)) + int(row.get("locked", 0))
                totals[row["currency"]] = totals.get(row["currency"], 0) + amount
        return totals

    # -- main loop -----------------------------------------------------------
    def run(self, steps):
        """
        Take one action per step, cycling through the markets in turn.

        Round-robin rather than a random market each step: coverage should not
        depend on the seed, and an even split makes the per-market numbers in
        the report comparable to each other.
        """
        order = list(self.sims.values())

        for step in range(steps):
            sim = order[step % len(order)]
            sim.step_price()

            roll = self.rng.random()
            if roll < 0.45 and self.makers:
                sim.act_maker(self.rng.choice(self.makers))
            elif roll < 0.75 and self.takers:
                sim.act_taker(self.rng.choice(self.takers))
            elif roll < 0.97 and self.retail:
                sim.act_retail(self.rng.choice(self.retail))
            elif self.takers:
                # A whale. Rare by design — this is the event that produces
                # multi-level fills and a visible mark on the tape.
                whale = self.rng.choice(self.takers)
                sim.log(whale.name, "WHALE", "sweeping the book")
                sim.act_taker(whale, aggression=self.rng.randint(3, 6))

            # Settlement runs behind the 202, so give it a moment before the
            # tape is read — otherwise every trade is reported one step late.
            time.sleep(self.args.tick)
            sim.drain_tape()

        # Trades from the last action on each market, and anything the
        # round-robin left unread on the others.
        for sim in order:
            sim.drain_tape()

    # -- settlement latency --------------------------------------------------
    def probe_settlement(self, rounds):
        """
        Measure the path the 202 hides: match, ledger event, settled, readable.

        Each probe rests a known order, crosses it, and polls the public tape
        until a new trade id appears. That covers the matching worker, the
        outbox append, the settlement worker's transaction and the read back —
        everything between the acknowledgement and the execution being a fact
        the rest of the world can see.

        Run with nothing else in flight, so the trade that appears is the one
        this probe caused. The figure still carries one poll interval of
        granularity, which is why the interval is small and the number is
        reported as a distribution rather than a single value.
        """
        if rounds <= 0:
            return []

        samples = []
        if not self.makers or not self.takers:
            return samples

        for round_no in range(rounds):
            sim = list(self.sims.values())[round_no % len(self.sims)]
            maker = self.makers[round_no % len(self.makers)]
            taker = self.takers[round_no % len(self.takers)]

            # Rest one tick above the best bid, so nothing already on the book
            # crosses it and the only thing that can fill it is the taker below.
            book = sim.api.book(sim.symbol, levels=1)
            bids = book["bids"]
            rest_price = sim.round_tick(bids[0]["price"] + sim.tick) if bids else sim.round_tick(sim.fair)
            quantity = sim.lot * 4

            if sim.place(maker, "sell", quantity, rest_price, tag="probe") is None:
                continue

            try:
                tape = sim.api.tape(sim.symbol, limit=1)
            except ApiError:
                continue
            before = tape[0]["id"] if tape else 0  # newest, whoever put it there

            started = time.perf_counter()
            if sim.place(taker, "buy", quantity, rest_price, tag="probe") is None:
                continue

            deadline = started + self.args.probe_timeout
            while time.perf_counter() < deadline:
                try:
                    tape = sim.api.tape(sim.symbol, limit=1)
                except ApiError:
                    break
                if tape and tape[0]["id"] != before:
                    samples.append((time.perf_counter() - started) * 1000)
                    break
                time.sleep(0.005)

            sim.drain_tape()

        return samples

    # -- throughput ----------------------------------------------------------
    def burst(self, count, workers):
        """
        Measure how many orders a second the intake path sustains.

        The orders are passive and priced far from the touch, so they rest
        rather than match: this is a measurement of POST /orders — the balance
        update, the order insert and the queue send — and not of the matching
        engine, which runs behind the response and would otherwise be timed
        through a queue the client cannot see.

        Everything placed is then cancelled, which measures DELETE /orders/{id}
        the same way and leaves the book as it found it. The cancellations are
        acknowledgements, not completions: the release is settled asynchronously,
        exactly as in production.
        """
        if count <= 0 or not self.traders:
            return None

        sims = list(self.sims.values())
        tasks = []
        for i in range(count):
            sim = sims[i % len(sims)]
            trader = self.traders[i % len(self.traders)]
            # Half the fair value on the bid, double it on the ask. Nothing
            # resting is anywhere near either, so none of this can match.
            if i % 2 == 0:
                side, price = "buy", sim.round_tick(max(sim.tick, sim.fair // 2))
            else:
                side, price = "sell", sim.round_tick(sim.fair * 2)
            tasks.append((trader, sim, side, sim.lot, price))

        def submit(task):
            trader, sim, side, quantity, price = task
            try:
                ack = self.api.place(trader.token, sim.symbol, side, quantity, price)
                return trader, ack["order_id"]
            except (ApiError, SystemExit):
                return trader, None

        started = time.perf_counter()
        with concurrent.futures.ThreadPoolExecutor(max_workers=workers) as pool:
            results = list(pool.map(submit, tasks))
        place_elapsed = time.perf_counter() - started

        accepted = [(trader, order_id) for trader, order_id in results if order_id]

        def retract(entry):
            trader, order_id = entry
            try:
                self.api.cancel(trader.token, order_id)
                return True
            except (ApiError, SystemExit):
                return False

        started = time.perf_counter()
        with concurrent.futures.ThreadPoolExecutor(max_workers=workers) as pool:
            retracted = list(pool.map(retract, accepted))
        cancel_elapsed = time.perf_counter() - started

        return {
            "requested": count,
            "workers": workers,
            "accepted": len(accepted),
            "place_seconds": place_elapsed,
            "cancelled": sum(1 for ok in retracted if ok),
            "cancel_seconds": cancel_elapsed,
        }

    # -- report --------------------------------------------------------------
    def report(self, probes, burst, wall):
        rule = "─" * 78
        print("\n" + rule)
        print(f"  {len(self.sims)} market(s), {len(self.traders)} accounts, "
              f"{wall:.1f}s wall clock")
        print(rule)

        self.report_throughput(burst, wall)
        self.report_latency()
        self.report_settlement(probes)
        for sim in self.sims.values():
            self.report_market(sim)
        self.report_accounts()
        self.report_conservation()
        print()

    def report_throughput(self, burst, wall):
        placed = self.metrics.get("placed")
        rejected = self.metrics.get("rejected")
        cancelled = self.metrics.get("cancelled")
        trades = sum(sim.trades for sim in self.sims.values())
        requests = self.metrics.requests()

        print("\n  throughput")
        print(f"    {'HTTP requests':<30} {requests:>9,}   {requests / wall:>8.1f}/s")
        print(f"    {'orders accepted':<30} {placed:>9,}   {placed / wall:>8.1f}/s")
        print(f"    {'orders rejected':<30} {rejected:>9,}   "
              f"{100 * rejected / max(1, placed + rejected):>7.1f}%")
        print(f"    {'cancellations accepted':<30} {cancelled:>9,}   "
              f"{cancelled / wall:>8.1f}/s")
        print(f"    {'cancellations lost to a fill':<30} "
              f"{self.metrics.get('cancel_lost_race'):>9,}")
        print(f"    {'executions settled':<30} {trades:>9,}   {trades / wall:>8.1f}/s")

        # The steady loop is paced by --tick, so its rate is a setting rather
        # than a finding. The burst is the measurement.
        if burst is None:
            print("\n    (no burst measured — pass --burst N for a rate under concurrency)")
            return

        place_rate = burst["accepted"] / burst["place_seconds"] if burst["place_seconds"] else 0
        cancel_rate = burst["cancelled"] / burst["cancel_seconds"] if burst["cancel_seconds"] else 0

        print(f"\n  burst, {burst['workers']} concurrent clients "
              f"(passive orders — intake only, nothing matches)")
        print(f"    {'POST /orders':<30} {burst['accepted']:>9,}   {place_rate:>8.1f}/s"
              f"   ({burst['requested'] - burst['accepted']} rejected)")
        print(f"    {'DELETE /orders/{id}':<30} {burst['cancelled']:>9,}   "
              f"{cancel_rate:>8.1f}/s")

    def report_latency(self):
        print("\n  latency, milliseconds  (client-side, around the HTTP call)")
        print(f"    {'endpoint':<14} {'n':>7}  {'p50':>8} {'p95':>8} {'p99':>8} "
              f"{'max':>8}  {'fail':>5}")

        order = ["place", "cancel", "book", "tape", "ticker", "wallet",
                 "orders", "deposit", "login", "signup", "markets", "currencies"]
        labels = [k for k in order if k in self.metrics.latency]
        labels += [k for k in sorted(self.metrics.latency) if k not in order]

        for label in labels:
            samples = self.metrics.latency[label]
            print(f"    {label:<14} {len(samples):>7,}  "
                  f"{percentile(samples, 50):>8.1f} {percentile(samples, 95):>8.1f} "
                  f"{percentile(samples, 99):>8.1f} {max(samples):>8.1f}  "
                  f"{self.metrics.failures.get(label, 0):>5,}")

        # Two of those failures are the run working correctly: a 409 on cancel
        # is an order that filled first, and a rejected signup is an account
        # this script already created on an earlier run.
        print("    (fail counts 4xx as well as 5xx — see 'lost to a fill' above)")

    def report_settlement(self, probes):
        print("\n  settlement")

        if probes:
            print(f"    202 to readable over REST, {len(probes)} probe(s)")
            print(f"      p50 {percentile(probes, 50):.1f} ms   "
                  f"p95 {percentile(probes, 95):.1f} ms   "
                  f"max {max(probes):.1f} ms")
        else:
            print("    no probes taken (--probes 0, or nothing crossed)")

        lags = [lag for sim in self.sims.values() for lag in sim.lags]
        if lags:
            # Upper bound, not a measurement: it counts however long the loop
            # waited before reading the tape.
            print(f"    match to first read by this script, {len(lags)} execution(s)")
            print(f"      p50 {percentile(lags, 50):.0f} ms   "
                  f"p95 {percentile(lags, 95):.0f} ms")
            print("      Not a server figure. Settlement is its floor; the tail is "
                  "this script\n      waiting its turn in the round-robin before "
                  "reading that market's tape.")

    def report_market(self, sim):
        print(f"\n  {sim.symbol}  —  {sim.placed} orders, {sim.cancelled} cancellations, "
              f"{sim.rejected} rejections")

        ticker = sim.api.ticker(sim.symbol)
        if ticker["has_traded"]:
            change = ticker["change"]
            pct = (change / ticker["open_price"] * 100) if ticker["open_price"] else 0
            print(f"    last {sim.price_str(ticker['last_price'])}   "
                  f"open {sim.price_str(ticker['open_price'])}   "
                  f"change {'+' if change >= 0 else ''}{sim.price_str(change)} "
                  f"({pct:+.2f}%)")
            print(f"    high {sim.price_str(ticker['high'])}   "
                  f"low  {sim.price_str(ticker['low'])}   "
                  f"volume {sim.qty_str(ticker['base_volume'])} {sim.base}   "
                  f"trades {ticker['trade_count']}")
        else:
            print("    nothing traded — the book never crossed")

        if sim.trades:
            print(f"    seen here: {sim.trades} execution(s), "
                  f"{sim.qty_str(sim.base_volume)} {sim.base} "
                  f"/ {fmt(sim.quote_volume, sim.quote_exp)} {sim.quote} notional")
            print(f"    aggressor: {sim.taker_buy} buy / {sim.taker_sell} sell   "
                  f"average size {sim.qty_str(sim.base_volume // sim.trades)} {sim.base}")

        book = sim.api.book(sim.symbol, levels=6)
        bids, asks = book["bids"], book["asks"]

        if bids and asks:
            spread = asks[0]["price"] - bids[0]["price"]
            mid = (asks[0]["price"] + bids[0]["price"]) // 2
            bid_depth = sum(level["quantity"] for level in bids)
            ask_depth = sum(level["quantity"] for level in asks)
            print(f"    spread {sim.price_str(spread)} "
                  f"({spread * 10_000 // max(1, mid)} bps)   "
                  f"top-6 depth {sim.qty_str(bid_depth)} bid / "
                  f"{sim.qty_str(ask_depth)} ask")

        for level in reversed(asks):
            print(f"      ask {sim.price_str(level['price']):>14}  "
                  f"{sim.qty_str(level['quantity']):>14}  ({level['orders']})")
        if bids and asks:
            print(f"      {'— touch —':^42}")
        for level in bids:
            print(f"      bid {sim.price_str(level['price']):>14}  "
                  f"{sim.qty_str(level['quantity']):>14}  ({level['orders']})")

    def report_accounts(self):
        print("\n  accounts")

        codes = sorted({code for sim in self.sims.values()
                        for code in (sim.base, sim.quote)})

        for trader in self.traders:
            balances = {b["currency"]: b for b in self.api.wallet(trader.token)["balances"]}
            orders = self.api.orders(trader.token)
            resting = sum(1 for o in orders if o["status"] in ("open", "partially_filled"))
            filled = sum(1 for o in orders if o["status"] == "filled")
            partial = sum(1 for o in orders if o["status"] == "partially_filled")

            held = "  ".join(
                f"{fmt(int(balances.get(code, {}).get('available', 0)) + int(balances.get(code, {}).get('locked', 0)), self.exponents[code], 14)} {code}"
                for code in codes
            )
            print(f"    {trader.name:<10} {trader.role:<7} {held}")
            print(f"    {'':<10} {'':<7} resting {resting:>3}  filled {filled:>3}  "
                  f"partial {partial:>3}")

    def report_conservation(self):
        """
        The check the docstring's first SQL query makes from the outside.

        Trading moves money between these accounts; it never mints any. So the
        roster's total in each currency must be exactly what it was before the
        first order. A difference means either settlement moved money it should
        not have, or somebody outside this script traded with it — in which case
        rerun against a database only the simulator is using.
        """
        print("\n  conservation  (roster totals, before → after)")

        final = self.holdings()
        codes = sorted(set(self.baseline) | set(final))
        clean = True

        for code in codes:
            before = self.baseline.get(code, 0)
            after = final.get(code, 0)
            exponent = self.exponents.get(code, 0)
            delta = after - before
            if delta:
                clean = False
            mark = "ok" if delta == 0 else f"OFF BY {fmt(delta, exponent)}"
            print(f"    {code:<5} {fmt(before, exponent, 20)} → "
                  f"{fmt(after, exponent, 20)}   {mark}")

        if not clean:
            print("\n    A non-zero difference is a real finding. Check for events that "
                  "\n    never applied:  SELECT * FROM ledger_events WHERE failed_at IS NOT NULL;")


def resolve_prices(markets, requested):
    """
    Work out a starting price for every market.

    --price takes either a bare number, which becomes the default for anything
    not named, or SYMBOL=N pairs, and may be repeated. Unnamed markets fall back
    to the table at the top of this file, because an opening mark three orders
    of magnitude out would make tick size and order size nonsense on that book.
    """
    default = None
    explicit = {}

    for entry in requested or []:
        if "=" in entry:
            symbol, _, value = entry.partition("=")
            explicit[symbol.strip()] = int(value)
        else:
            default = int(entry)

    prices = {}
    for market in markets:
        symbol = market["symbol"]
        if symbol in explicit:
            prices[symbol] = explicit[symbol]
        elif default is not None:
            prices[symbol] = default
        else:
            prices[symbol] = DEFAULT_PRICES.get(market["base"], FALLBACK_PRICE)

        if prices[symbol] <= 0:
            raise SystemExit(f"starting price for {symbol} must be positive")

    unknown = set(explicit) - {m["symbol"] for m in markets}
    if unknown:
        raise SystemExit(f"--price names markets that are not listed: {', '.join(sorted(unknown))}")

    return prices


def select_markets(listed, requested):
    if requested in (None, "", "all"):
        return listed

    wanted = [part.strip() for part in requested.split(",") if part.strip()]
    by_symbol = {m["symbol"]: m for m in listed}

    missing = [symbol for symbol in wanted if symbol not in by_symbol]
    if missing:
        available = ", ".join(by_symbol)
        raise SystemExit(f"No market(s) {', '.join(missing)}. Listed: {available}")

    return [by_symbol[symbol] for symbol in wanted]


def main():
    parser = argparse.ArgumentParser(
        description="Generate market activity against a running exchange, and measure it.",
        formatter_class=argparse.ArgumentDefaultsHelpFormatter,
    )
    parser.add_argument("--api", default="http://localhost:8080", help="base URL of the API")
    parser.add_argument("--markets", default="all",
                        help="'all', or a comma-separated list of symbols")
    parser.add_argument("--market", default=None,
                        help="trade one symbol only (shorthand for --markets SYMBOL)")
    parser.add_argument("--steps", type=int, default=240, help="how many actions to take, across all markets")
    parser.add_argument("--tick", type=float, default=0.25, help="seconds between actions")
    parser.add_argument("--seed", type=int, default=None, help="seed for a reproducible run")
    parser.add_argument("--quiet", action="store_true",
                        help="suppress the per-action log and print only the report")

    parser.add_argument("--makers", type=int, default=2, help="two-sided quoting accounts")
    parser.add_argument("--takers", type=int, default=3, help="spread-crossing accounts")
    parser.add_argument("--retail", type=int, default=3, help="small noise accounts")

    parser.add_argument("--price", action="append", default=None, metavar="[SYMBOL=]WHOLE",
                        help="starting price in WHOLE quote units; repeatable, "
                             "bare number sets the default for every market")
    parser.add_argument("--vol", type=int, default=None, help="per-step noise, in quote minor units")
    parser.add_argument("--trend", type=int, default=None, help="drift magnitude, in quote minor units")
    parser.add_argument("--spread", type=int, default=None, help="maker spread, in quote minor units")
    parser.add_argument("--requote", type=int, default=None, help="price move that triggers a requote")
    parser.add_argument("--ladder", type=int, default=3, help="price levels a maker quotes per side")
    parser.add_argument("--grid", type=int, default=25, help="maker quotes round to this many ticks")

    parser.add_argument("--quote-size", type=int, default=None, help="maker size, in base minor units")
    parser.add_argument("--take-size", type=int, default=None, help="taker size, in base minor units")
    parser.add_argument("--retail-size", type=int, default=None, help="retail size, in base minor units")
    parser.add_argument("--inventory", type=int, default=40,
                        help="maker's target base holding per market, in WHOLE units")

    parser.add_argument("--probes", type=int, default=12,
                        help="settlement latency probes to take after the run (0 to skip)")
    parser.add_argument("--probe-timeout", type=float, default=5.0,
                        help="seconds to wait for a probed execution to become readable")
    parser.add_argument("--burst", type=int, default=200,
                        help="passive orders to fire concurrently for a throughput figure (0 to skip)")
    parser.add_argument("--burst-workers", type=int, default=8,
                        help="concurrent clients in the burst")

    args = parser.parse_args()
    rng = random.Random(args.seed)

    metrics = Metrics()
    api = Client(args.api, metrics)

    listed = api.markets()
    if not listed:
        raise SystemExit("The exchange lists no markets.")

    markets = select_markets(listed, args.market or args.markets)
    prices = resolve_prices(markets, args.price)
    exponents = {c["code"]: c["exponent"] for c in api.currencies()}

    for market in markets:
        for role in ("base", "quote"):
            if market[role] not in exponents:
                raise SystemExit(
                    f"{market['symbol']} names a {role} currency "
                    f"{market[role]!r} that GET /currencies does not list."
                )

    symbols = ", ".join(m["symbol"] for m in markets)
    print(f"Simulating {symbols} on {args.api}"
          f"{f' (seed {args.seed})' if args.seed is not None else ''}\n")

    exchange = Exchange(api, metrics, rng, args, markets, exponents, prices)
    exchange.setup()
    if not args.quiet:
        print()

    probes = []
    burst = None
    try:
        exchange.run(args.steps)
        probes = exchange.probe_settlement(args.probes)
        burst = exchange.burst(args.burst, args.burst_workers)
    except KeyboardInterrupt:
        print("\ninterrupted — summarising what happened so far", file=sys.stderr)

    exchange.report(probes, burst, metrics.elapsed())


if __name__ == "__main__":
    main()
