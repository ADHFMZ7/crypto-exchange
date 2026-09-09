import React, { useCallback, useState } from "react";
import { SourcedPanel } from "./DataSource";
import { StreamBadge } from "./StreamBadge";
import { useMarketStream } from "../hooks/useMarketStream";
import { usePolling } from "../hooks/usePolling";
import { useReference } from "../hooks/useReference";
import { api, errorMessage } from "../lib/api";
import { formatPrice, formatQuantity } from "../lib/markets";
import type { MarketTrade } from "../types";

const TRADES = 15;

type Props = {
  symbol: string | undefined;
};

/**
 * The public tape for one market, from GET /markets/{symbol}/trades.
 *
 * Colour follows the aggressor: a buy lifted an offer, a sell hit a bid. That
 * is the only thing the tape says about intent, and it is why `taker_side`
 * crosses the wire rather than being inferred from consecutive prices.
 *
 * Unauthenticated, like the book — nothing here is attributable to an account.
 */
export const TradeTape: React.FC<Props> = ({ symbol }) => {
  const { reference } = useReference();

  const [trades, setTrades] = useState<MarketTrade[]>([]);
  const [error, setError] = useState<string>();
  const [loaded, setLoaded] = useState(false);

  const load = useCallback(async () => {
    if (!symbol) return;
    try {
      const res = await api.getMarketTrades(symbol, TRADES);
      setTrades(res.trades ?? []);
      setError(undefined);
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setLoaded(true);
    }
  }, [symbol]);

  /*
   * Executions arrive on the feed rather than being asked for.
   *
   * The snapshot still comes from REST — a stream only carries what happens
   * after it connects — and is re-read on every reconnect, since the gap while
   * disconnected is the one window the feed cannot fill.
   *
   * Deduplicated by id because those two sources overlap: a trade can arrive on
   * the feed and then appear again in the snapshot that follows a reconnect.
   */
  const status = useMarketStream(
    symbol,
    (trade) =>
      setTrades((prev) =>
        prev.some((t) => t.id === trade.id) ? prev : [trade, ...prev].slice(0, TRADES)
      ),
    load
  );

  // Only while the feed is down. Falling back to asking is better than showing
  // a tape that has quietly stopped moving.
  usePolling(load, 4000, Boolean(symbol) && status !== "live");

  return (
    <SourcedPanel
      eyebrow="Tape"
      title="Recent trades"
      kind="live"
      endpoint="GET /markets/{symbol}/trades"
      fill
      actions={<StreamBadge status={status} />}
      note={<>Trades that have actually happened here, most recent first.</>}
      devNote={
        <>
          Colour is <code>taker_side</code> — the side that crossed the spread, not the side that
          profited.
        </>
      }
    >
      {error && <div className="pill status-danger">{error}</div>}

      {!symbol && <div className="muted">Pick a market to see its tape.</div>}

      {symbol && (
        <table className="table">
          <thead>
            <tr>
              <th>Time</th>
              <th style={{ textAlign: "right" }}>Price</th>
              <th style={{ textAlign: "right" }}>Quantity</th>
              <th>Taker</th>
            </tr>
          </thead>
          <tbody>
            {trades.map((trade) => (
              <tr key={trade.id}>
                <td className="muted">{new Date(trade.executed_at).toLocaleTimeString()}</td>
                <td
                  style={{
                    textAlign: "right",
                    color: trade.taker_side === "buy" ? "var(--success)" : "var(--danger)"
                  }}
                >
                  {formatPrice(reference, symbol, trade.price)}
                </td>
                <td style={{ textAlign: "right" }}>
                  {formatQuantity(reference, symbol, trade.quantity)}
                </td>
                <td className="muted">{trade.taker_side}</td>
              </tr>
            ))}

            {loaded && !trades.length && (
              <tr>
                <td colSpan={4} className="muted">
                  Nothing has traded on this market yet.
                </td>
              </tr>
            )}
          </tbody>
        </table>
      )}
    </SourcedPanel>
  );
};
