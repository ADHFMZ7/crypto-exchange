import React, { useCallback, useState } from "react";
import { Link } from "react-router-dom";
import { SourcedPanel } from "../components/DataSource";
import { useAuth } from "../hooks/useAuth";
import { usePolling } from "../hooks/usePolling";
import { useReference } from "../hooks/useReference";
import { ApiError, api, errorMessage } from "../lib/api";
import { toAmount } from "../lib/decimal";
import {
  effectiveExponent,
  formatAmount,
  formatPrice,
  formatQuantity,
  marketsFor,
  minorUnitName,
  percentChange,
  roleIn
} from "../lib/markets";
import type { Ticker, WalletBalance } from "../types";

/**
 * What the exchange lists, and what you hold of it.
 *
 * The registry is the authority on both — the frontend keeps no table of its
 * own — so this page is the readable form of `GET /currencies` and
 * `GET /markets`, joined to the ticker and to your balances.
 *
 * It exists because a currency you hold none of is otherwise invisible: the
 * wallet shows what you have, and a new account has USD and nothing else. That
 * is a poor way to discover that three other currencies are tradeable.
 */
export const MarketsPage: React.FC = () => {
  const { token, logout } = useAuth();
  const { reference } = useReference();

  const [tickers, setTickers] = useState<Ticker[]>([]);
  const [balances, setBalances] = useState<WalletBalance[]>([]);
  const [error, setError] = useState<string>();

  const load = useCallback(async () => {
    try {
      const res = await api.getTickers();
      setTickers(res.tickers ?? []);
      setError(undefined);
    } catch (err) {
      setError(errorMessage(err));
    }

    if (!token) return;
    try {
      const wallet = await api.getWallet(token);
      setBalances(wallet.balances ?? []);
    } catch (err) {
      if (err instanceof ApiError && err.isUnauthorized) logout();
    }
  }, [logout, token]);

  usePolling(load, 8000);

  const codes = Object.keys(reference.currencies).sort();

  const held = (code: string) => {
    const balance = balances.find((b) => b.currency === code);
    return {
      available: balance ? toAmount(balance.available) : 0n,
      locked: balance ? toAmount(balance.locked) : 0n
    };
  };

  return (
    <div className="grid" style={{ gap: 18 }}>
      <SourcedPanel
        eyebrow="Reference"
        title="Currencies"
        kind="live"
        endpoint="GET /currencies"
        note={<>Everything you can hold here, and what you currently hold of it.</>}
        devNote={
          <>
            Precision is what a currency's smallest unit is worth — every amount in this app is an
            integer count of those, never a decimal. Nothing is hardcoded in the frontend: a second
            copy that disagreed with the backend would be a factor-of-10<sup>n</sup> error in every
            figure on every screen.
          </>
        }
      >
        {error && <div className="pill status-danger">{error}</div>}

        <table className="table">
          <thead>
            <tr>
              <th>Currency</th>
              <th>Smallest unit</th>
              <th style={{ textAlign: "right" }}>Available</th>
              <th style={{ textAlign: "right" }}>Locked</th>
              <th>Trades in</th>
            </tr>
          </thead>
          <tbody>
            {codes.map((code) => {
              const currency = reference.currencies[code];
              const exponent = effectiveExponent(reference, code);
              const holding = held(code);
              const markets = marketsFor(reference, code);

              return (
                <tr key={code}>
                  <td>
                    <strong>{code}</strong>
                    <div className="muted" style={{ fontSize: 12 }}>
                      {currency?.name ?? code}
                    </div>
                  </td>
                  <td>
                    <code>{minorUnitName(code, exponent)}</code>
                    <div className="muted" style={{ fontSize: 12 }}>
                      {exponent === 0 ? "whole units only" : `${exponent} decimal places`}
                    </div>
                  </td>
                  <td style={{ textAlign: "right" }} className={holding.available ? undefined : "muted"}>
                    {formatAmount(reference, holding.available, code)}
                  </td>
                  <td style={{ textAlign: "right" }} className={holding.locked ? undefined : "muted"}>
                    {formatAmount(reference, holding.locked, code)}
                  </td>
                  <td>
                    {markets.length === 0 ? (
                      <span className="muted">no markets</span>
                    ) : (
                      <div className="inline-actions" style={{ gap: 6, flexWrap: "wrap" }}>
                        {markets.map((m) => (
                          <span key={m.symbol} className="tag" title={`${code} is the ${roleIn(m, code)}`}>
                            {m.symbol}
                            <span className="muted" style={{ textTransform: "none" }}>
                              {roleIn(m, code)}
                            </span>
                          </span>
                        ))}
                      </div>
                    )}
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </SourcedPanel>

      <SourcedPanel
        eyebrow="Reference"
        title="Markets"
        kind="live"
        endpoint="GET /markets · GET /markets/tickers"
        note={
          <>
            Each market pairs two currencies. You trade an amount of the first, priced in the
            second.
          </>
        }
        devNote={
          <>
            Amounts are denominated in the <strong>base</strong>, prices and totals in the{" "}
            <strong>quote</strong>. With no external feed, an untraded market has no price at all
            rather than a stale one.
          </>
        }
      >
        <table className="table">
          <thead>
            <tr>
              <th>Market</th>
              <th>Base</th>
              <th>Quote</th>
              <th style={{ textAlign: "right" }}>Last</th>
              <th style={{ textAlign: "right" }}>Change</th>
              <th style={{ textAlign: "right" }}>Volume</th>
              <th style={{ textAlign: "right" }}>Trades</th>
              <th />
            </tr>
          </thead>
          <tbody>
            {reference.markets.map((market) => {
              const ticker = tickers.find((t) => t.market === market.symbol);
              const percent = ticker ? percentChange(ticker) : undefined;
              const up = (ticker?.change ?? 0) >= 0;

              return (
                <tr key={market.symbol}>
                  <td>
                    <code>{market.symbol}</code>
                  </td>
                  <td>
                    {market.base}{" "}
                    <span className="muted">({effectiveExponent(reference, market.base)}dp)</span>
                  </td>
                  <td>
                    {market.quote}{" "}
                    <span className="muted">({effectiveExponent(reference, market.quote)}dp)</span>
                  </td>

                  {ticker?.has_traded ? (
                    <>
                      <td style={{ textAlign: "right" }}>
                        {formatPrice(reference, market.symbol, ticker.last_price)}
                      </td>
                      <td
                        style={{
                          textAlign: "right",
                          color: up ? "var(--success)" : "var(--danger)"
                        }}
                      >
                        {percent === undefined
                          ? "—"
                          : `${percent >= 0 ? "+" : ""}${percent.toFixed(2)}%`}
                      </td>
                      <td style={{ textAlign: "right" }}>
                        {formatQuantity(reference, market.symbol, ticker.base_volume)}
                      </td>
                      <td style={{ textAlign: "right" }}>{ticker.trade_count.toLocaleString()}</td>
                    </>
                  ) : (
                    <td className="muted" colSpan={4} style={{ textAlign: "right" }}>
                      nothing has traded here yet
                    </td>
                  )}

                  <td style={{ textAlign: "right" }}>
                    <Link to="/trades/new">
                      <button type="button" className="ghost-button">
                        Trade
                      </button>
                    </Link>
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </SourcedPanel>
    </div>
  );
};
