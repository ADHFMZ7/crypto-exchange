import React, { useEffect, useMemo, useState } from "react";
import { Link } from "react-router-dom";
import { SourcedPanel } from "../components/DataSource";
import { useAuth } from "../hooks/useAuth";
import { useReference } from "../hooks/useReference";
import { ApiError, api, errorMessage } from "../lib/api";
import { fromMinorUnits, toAmount } from "../lib/decimal";
import {
  buildIntent,
  counterpartsFor,
  effectiveExponent,
  formatAmount,
  formatBalance,
  resolveMarkets,
  toTradePayload,
  tradeableCurrencies
} from "../lib/markets";
import { appendOrder } from "../lib/orderLog";
import type { TradeAck, WalletBalance } from "../types";

type Mode = "exchange" | "cancel";

export const CreateTradePage: React.FC = () => {
  const { token, logout } = useAuth();
  const { reference } = useReference();

  const [mode, setMode] = useState<Mode>("exchange");
  const [spendCurrency, setSpendCurrency] = useState("USD");
  const [receiveCurrency, setReceiveCurrency] = useState("BTC");
  const [spendAmount, setSpendAmount] = useState("45000");
  const [receiveAmount, setReceiveAmount] = useState("1");

  const [preferredSymbol, setPreferredSymbol] = useState<string | undefined>();
  const [cancelOrderID, setCancelOrderID] = useState("");
  const [balances, setBalances] = useState<WalletBalance[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string>();
  const [result, setResult] = useState<TradeAck | null>(null);

  const currencies = tradeableCurrencies(reference);

  // Reference data can arrive after first paint and change the market list.
  useEffect(() => {
    if (currencies.length && !currencies.includes(spendCurrency)) {
      setSpendCurrency(currencies[0]);
    }
  }, [currencies, spendCurrency]);

  useEffect(() => {
    if (!token) return;
    let cancelled = false;

    api
      .getWallet(token)
      .then((wallet) => {
        if (!cancelled) setBalances(wallet.balances ?? []);
      })
      .catch((err) => {
        if (!cancelled && err instanceof ApiError && err.isUnauthorized) logout();
      });

    return () => {
      cancelled = true;
    };
  }, [logout, token]);

  const availableOf = (code: string): bigint => {
    const balance = balances.find((b) => b.currency === code);
    return balance ? toAmount(balance.available) : 0n;
  };

  // A currency pair can back more than one market (spot vs. perp vs. dated).
  // With a single candidate the user never sees this.
  const candidates = useMemo(
    () => resolveMarkets(reference, spendCurrency, receiveCurrency),
    [receiveCurrency, reference, spendCurrency]
  );

  const built = useMemo(
    () =>
      buildIntent(
        reference,
        spendCurrency,
        spendAmount,
        receiveCurrency,
        receiveAmount,
        preferredSymbol
      ),
    [preferredSymbol, receiveAmount, receiveCurrency, reference, spendAmount, spendCurrency]
  );

  const intent = "intent" in built ? built.intent : null;
  const intentError = "error" in built ? built.error : null;

  const spendAvailableMinor = availableOf(spendCurrency);
  const shortfall = intent ? intent.spendMinor - spendAvailableMinor : 0n;
  const insufficient = shortfall > 0n;

  const swap = () => {
    setSpendCurrency(receiveCurrency);
    setReceiveCurrency(spendCurrency);
    setSpendAmount(receiveAmount);
    setReceiveAmount(spendAmount);
  };

  const setMax = () => {
    setSpendAmount(fromMinorUnits(spendAvailableMinor, effectiveExponent(reference, spendCurrency)));
  };

  /**
   * Settles both fields onto the values actually being sent. Runs on blur rather
   * than on change so rounding never fights someone mid-keystroke.
   */
  const normalizeAmounts = () => {
    if (!intent) return;
    setSpendAmount(
      fromMinorUnits(intent.spendMinor, effectiveExponent(reference, intent.spendCurrency))
    );
    setReceiveAmount(
      fromMinorUnits(intent.receiveMinor, effectiveExponent(reference, intent.receiveCurrency))
    );
  };

  const onSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!token) {
      setError("You must be logged in.");
      return;
    }

    setLoading(true);
    setError(undefined);
    setResult(null);

    try {
      let ack: TradeAck;

      if (mode === "cancel") {
        const parsedID = Number(cancelOrderID);
        if (!Number.isInteger(parsedID) || parsedID <= 0) {
          setError("Cancel requests need a positive order ID.");
          return;
        }
        ack = await api.submitTrade(token, {
          market: reference.markets[0]?.symbol ?? "BTC-USD",
          type: "cancel",
          order_id: parsedID
        });
        setCancelOrderID("");
        appendOrder({
          order_id: ack.order_id,
          market: ack.market,
          type: "cancel",
          shares: null,
          price: null,
          submittedAt: ack.receivedAt
        });
      } else {
        if (!intent) {
          setError(intentError?.message ?? "Fill in both amounts.");
          return;
        }
        ack = await api.submitTrade(token, toTradePayload(intent));
        appendOrder({
          order_id: ack.order_id,
          market: ack.market,
          type: intent.side === "buy" ? "limit_buy" : "limit_sell",
          shares: Number(intent.quantity),
          price: Number(intent.price),
          submittedAt: ack.receivedAt
        });
      }

      setResult(ack);
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setLoading(false);
    }
  };

  const precisionHint = (code: string) => {
    const exponent = effectiveExponent(reference, code);
    return exponent === 0 ? "whole numbers only" : `up to ${exponent} decimals`;
  };

  return (
    <div className="grid" style={{ gap: 18 }}>
      <SourcedPanel
        eyebrow="Trade API"
        title="Exchange currency"
        kind="live"
        endpoint="POST /trades"
        note={
          <>
            Describe the trade as what you give and what you get — the market and side are worked out
            below. A <strong>202</strong> means the request was queued for the order book, not that it
            filled.
          </>
        }
        actions={
          <div className="inline-actions" style={{ gap: 6 }}>
            <button
              type="button"
              className={mode === "exchange" ? undefined : "ghost-button"}
              onClick={() => setMode("exchange")}
            >
              Exchange
            </button>
            <button
              type="button"
              className={mode === "cancel" ? undefined : "ghost-button"}
              onClick={() => setMode("cancel")}
            >
              Cancel order
            </button>
          </div>
        }
      >
        <form className="stack" style={{ gap: 14 }} onSubmit={onSubmit}>
          {mode === "cancel" ? (
            <label className="stack">
              <span>Order ID to cancel</span>
              <input
                type="number"
                min={1}
                step={1}
                value={cancelOrderID}
                onChange={(e) => setCancelOrderID(e.target.value)}
                placeholder="e.g. 42"
                required
              />
            </label>
          ) : (
            <>
              <div className="leg">
                <div className="leg-label">
                  <span>You spend</span>
                  <span className="muted">
                    Balance: {formatAmount(reference, availableOf(spendCurrency), spendCurrency)}
                    <button type="button" className="link-button" onClick={setMax}>
                      Max
                    </button>
                  </span>
                </div>
                <div className="leg-row">
                  <input
                    className="leg-amount"
                    type="text"
                    inputMode="decimal"
                    value={spendAmount}
                    onChange={(e) => setSpendAmount(e.target.value)}
                    onBlur={normalizeAmounts}
                    aria-label={`Amount of ${spendCurrency} to spend`}
                  />
                  <select
                    className="leg-currency"
                    value={spendCurrency}
                    onChange={(e) => {
                      const next = e.target.value;
                      setSpendCurrency(next);
                      if (next === receiveCurrency) {
                        setReceiveCurrency(counterpartsFor(reference, next)[0] ?? receiveCurrency);
                      }
                    }}
                    aria-label="Currency to spend"
                  >
                    {currencies.map((code) => (
                      <option key={code} value={code}>
                        {code} — {reference.currencies[code]?.name ?? code}
                      </option>
                    ))}
                  </select>
                </div>
                <div className="muted leg-hint">{precisionHint(spendCurrency)}</div>
              </div>

              <div className="swap-row">
                <button type="button" className="icon-button" onClick={swap} aria-label="Swap currencies">
                  ⇅
                </button>
              </div>

              <div className="leg">
                <div className="leg-label">
                  <span>You receive</span>
                  <span className="muted">
                    Balance: {formatAmount(reference, availableOf(receiveCurrency), receiveCurrency)}
                  </span>
                </div>
                <div className="leg-row">
                  <input
                    className="leg-amount"
                    type="text"
                    inputMode="decimal"
                    value={receiveAmount}
                    onChange={(e) => setReceiveAmount(e.target.value)}
                    onBlur={normalizeAmounts}
                    aria-label={`Amount of ${receiveCurrency} to receive`}
                  />
                  <select
                    className="leg-currency"
                    value={receiveCurrency}
                    onChange={(e) => {
                      const next = e.target.value;
                      setReceiveCurrency(next);
                      if (next === spendCurrency) {
                        setSpendCurrency(counterpartsFor(reference, next)[0] ?? spendCurrency);
                      }
                    }}
                    aria-label="Currency to receive"
                  >
                    {currencies.map((code) => (
                      <option key={code} value={code}>
                        {code} — {reference.currencies[code]?.name ?? code}
                      </option>
                    ))}
                  </select>
                </div>
                <div className="muted leg-hint">{precisionHint(receiveCurrency)}</div>
              </div>

              {intent && (
                <>
                  <div className="rate-line">
                    <span>
                      Limit rate <strong>1 {intent.market.base}</strong> ={" "}
                      <strong>{intent.rateDisplay}</strong>
                    </span>
                    <span className="muted">
                      {intent.side === "buy" ? "Spend at most" : "Receive at least"}{" "}
                      {formatAmount(
                        reference,
                        intent.side === "buy" ? intent.spendMinor : intent.receiveMinor,
                        intent.market.quote
                      )}
                    </span>
                  </div>

                  {intent.adjustment && (
                    <div className="adjust-note">
                      {intent.adjustment.fromPrecision
                        ? `${intent.adjustment.currency} holds ${effectiveExponent(reference, intent.adjustment.currency)} decimals, so that was rounded to `
                        : "Nudged to "}
                      <strong>
                        {formatAmount(
                          reference,
                          intent.adjustment.actual,
                          intent.adjustment.currency
                        )}
                      </strong>{" "}
                      (from{" "}
                      {formatAmount(reference, intent.adjustment.typed, intent.adjustment.currency)})
                      so the rate lands on a whole unit.
                    </div>
                  )}

                  {candidates.length > 1 ? (
                    <label className="stack" style={{ gap: 6 }}>
                      <span className="muted">
                        {spendCurrency}/{receiveCurrency} trades on more than one market — pick one:
                      </span>
                      <select
                        value={intent.market.symbol}
                        onChange={(e) => setPreferredSymbol(e.target.value)}
                      >
                        {candidates.map(({ market }) => (
                          <option key={market.symbol} value={market.symbol}>
                            {market.symbol}
                          </option>
                        ))}
                      </select>
                    </label>
                  ) : (
                    <div className="muted">
                      Routing via <code>{intent.market.symbol}</code>, chosen automatically from the
                      currency pair.
                    </div>
                  )}
                </>
              )}

              {intentError && <div className="pill status-danger">{intentError.message}</div>}

              {insufficient && intent && (
                <div className="pill status-danger">
                  Short {formatAmount(reference, shortfall, intent.spendCurrency)} — you have{" "}
                  {formatAmount(reference, availableOf(intent.spendCurrency), intent.spendCurrency)}.
                </div>
              )}

              {intent && (
                <details className="translation">
                  <summary>What gets sent to the matching engine</summary>
                  <table className="table" style={{ marginTop: 8 }}>
                    <tbody>
                      <tr>
                        <td className="muted">Market</td>
                        <td>
                          <code>{intent.market.symbol}</code>{" "}
                          <span className="muted">
                            base {intent.market.base} / quote {intent.market.quote}
                          </span>
                        </td>
                      </tr>
                      <tr>
                        <td className="muted">Side</td>
                        <td>
                          <code>{intent.side === "buy" ? "limit_buy" : "limit_sell"}</code>{" "}
                          <span className="muted">
                            you are {intent.side === "buy" ? "acquiring" : "giving up"}{" "}
                            {intent.market.base}
                          </span>
                        </td>
                      </tr>
                      <tr>
                        <td className="muted">shares</td>
                        <td>
                          <code>{intent.quantity.toString()}</code>{" "}
                          <span className="muted">
                            {effectiveExponent(reference, intent.market.base) === 0
                              ? `whole ${intent.market.base}`
                              : `${intent.market.base} minor units`}
                          </span>
                        </td>
                      </tr>
                      <tr>
                        <td className="muted">price</td>
                        <td>
                          <code>{intent.price.toString()}</code>{" "}
                          <span className="muted">
                            {effectiveExponent(reference, intent.market.quote) === 0
                              ? `${intent.market.quote} per ${intent.market.base}`
                              : `${intent.market.quote} minor units per whole ${intent.market.base}`}
                          </span>
                        </td>
                      </tr>
                    </tbody>
                  </table>
                </details>
              )}
            </>
          )}

          {error && <div className="pill status-danger">{error}</div>}

          <button type="submit" disabled={loading || (mode === "exchange" && !intent)}>
            {loading
              ? "Submitting…"
              : mode === "cancel"
                ? "Cancel order"
                : intent
                  ? `Spend ${formatAmount(reference, intent.spendMinor, intent.spendCurrency)}`
                  : "Submit request"}
          </button>

          {mode === "exchange" && (
            <div className="muted">
              The balances above go stale the moment an order is accepted — the server locks funds
              when it writes the order, so reload the wallet to see <code>locked</code> move. A{" "}
              <strong>202</strong> is not proof that happened: it is returned before the outcome of
              the lock is known.
            </div>
          )}
        </form>
      </SourcedPanel>

      <ReferenceStatus />

      {result && (
        <SourcedPanel
          eyebrow="Response"
          title="Accepted by the server"
          kind="live"
          endpoint="POST /trades"
          note={
            <>
              Saved to this browser's order log so it survives navigation.{" "}
              <Link to="/trades">View all submissions →</Link>
            </>
          }
        >
          <div className="inline-actions" style={{ gap: 12 }}>
            <div className="card">
              <div className="muted">Order ID</div>
              <strong>{result.order_id}</strong>
            </div>
            <div className="card">
              <div className="muted">Market</div>
              <strong>{result.market}</strong>
            </div>
            <div className="card">
              <div className="muted">Type</div>
              <strong>{result.type}</strong>
            </div>
            <div className="card">
              <div className="muted">Status</div>
              <strong className="status-success">{result.status}</strong>
            </div>
          </div>
          <div className="muted" style={{ marginTop: 12 }}>
            Received at {new Date(result.receivedAt).toLocaleString()}
          </div>
        </SourcedPanel>
      )}
    </div>
  );
};

/** Shows the currency and market table the app is running against. */
const ReferenceStatus: React.FC = () => {
  const { reference } = useReference();

  return (
    <SourcedPanel
      eyebrow="Reference data"
      title="Currencies and markets"
      kind="live"
      endpoint="GET /currencies, GET /markets"
      note={
        <>
          Served by the backend, so the exponents below are authoritative — the frontend keeps no
          copy of them. <strong>Every amount sent is integer minor units</strong>, cents and
          satoshis; dollars and bitcoin exist only in the fields above. Still outstanding on the
          server: <code>POST /trades</code> must read <code>shares</code> as satoshis and{" "}
          <code>price</code> as cents per whole BTC. See <code>docs/api-todos.md</code> § 1c.
        </>
      }
    >
      <table className="table">
        <thead>
          <tr>
            <th>Market</th>
            <th>Base</th>
            <th>Quote</th>
            <th style={{ textAlign: "right" }}>Precision</th>
          </tr>
        </thead>
        <tbody>
          {reference.markets.map((market) => (
            <tr key={market.symbol}>
              <td>
                <code>{market.symbol}</code>
              </td>
              <td>
                {market.base}{" "}
                <span className="muted">
                  ({effectiveExponent(reference, market.base)}dp)
                </span>
              </td>
              <td>
                {market.quote}{" "}
                <span className="muted">
                  ({effectiveExponent(reference, market.quote)}dp)
                </span>
              </td>
              <td style={{ textAlign: "right" }} className="muted">
                {effectiveExponent(reference, market.base) === 0 ? "whole units" : "minor units"}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </SourcedPanel>
  );
};
