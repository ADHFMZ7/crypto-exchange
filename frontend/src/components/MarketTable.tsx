import React, { useCallback, useState } from "react";
import { SourcedPanel } from "./DataSource";
import { usePolling } from "../hooks/usePolling";
import { useReference } from "../hooks/useReference";
import { api, errorMessage } from "../lib/api";
import { formatPrice, formatQuantity, percentChange } from "../lib/markets";
import type { Ticker } from "../types";

/**
 * The quote board, from GET /markets/tickers.
 *
 * Every market the registry lists appears, including ones that have never
 * traded — a market with no price is a fact about the exchange, and hiding it
 * would make an empty board look like a failed request.
 */
export const MarketTable: React.FC = () => {
  const { reference } = useReference();

  const [tickers, setTickers] = useState<Ticker[]>([]);
  const [error, setError] = useState<string>();
  const [loaded, setLoaded] = useState(false);

  const load = useCallback(async () => {
    try {
      const res = await api.getTickers();
      setTickers(res.tickers ?? []);
      setError(undefined);
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setLoaded(true);
    }
  }, []);

  usePolling(load, 8000);

  return (
    <SourcedPanel
      eyebrow="Markets"
      title="Quote board"
      kind="live"
      endpoint="GET /markets/tickers"
      note={
        <>
          Last traded price and the change over the trailing window, computed from the trades
          ledger. A market reads <strong>no trades yet</strong> until something actually crosses —
          there is no external price feed, so these move only when this exchange matches an order.
        </>
      }
    >
      {error && <div className="pill status-danger">{error}</div>}

      <table className="table">
        <thead>
          <tr>
            <th>Market</th>
            <th style={{ textAlign: "right" }}>Last</th>
            <th style={{ textAlign: "right" }}>Change</th>
            <th style={{ textAlign: "right" }}>High</th>
            <th style={{ textAlign: "right" }}>Low</th>
            <th style={{ textAlign: "right" }}>Volume</th>
            <th style={{ textAlign: "right" }}>Trades</th>
          </tr>
        </thead>
        <tbody>
          {tickers.map((t) => {
            const percent = percentChange(t);

            return (
              <tr key={t.market}>
                <td>{t.market}</td>

                {t.has_traded ? (
                  <>
                    <td style={{ textAlign: "right" }}>{formatPrice(reference, t.market, t.last_price)}</td>
                    <td
                      style={{
                        textAlign: "right",
                        color: t.change >= 0 ? "var(--success)" : "var(--danger)"
                      }}
                    >
                      {percent === undefined
                        ? "—"
                        : `${percent >= 0 ? "+" : ""}${percent.toFixed(2)}%`}
                    </td>
                    <td style={{ textAlign: "right" }}>{formatPrice(reference, t.market, t.high)}</td>
                    <td style={{ textAlign: "right" }}>{formatPrice(reference, t.market, t.low)}</td>
                    <td style={{ textAlign: "right" }}>
                      {formatQuantity(reference, t.market, t.base_volume)}
                    </td>
                    <td style={{ textAlign: "right" }}>{t.trade_count.toLocaleString()}</td>
                  </>
                ) : (
                  <td className="muted" colSpan={6} style={{ textAlign: "right" }}>
                    no trades yet
                  </td>
                )}
              </tr>
            );
          })}

          {loaded && !tickers.length && (
            <tr>
              <td className="muted" colSpan={7}>
                No markets listed.
              </td>
            </tr>
          )}
        </tbody>
      </table>
    </SourcedPanel>
  );
};
