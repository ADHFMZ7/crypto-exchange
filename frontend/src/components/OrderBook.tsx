import React, { useCallback, useState } from "react";
import { SourcedPanel } from "./DataSource";
import { usePolling } from "../hooks/usePolling";
import { useReference } from "../hooks/useReference";
import { api, errorMessage } from "../lib/api";
import { formatPrice, formatQuantity } from "../lib/markets";
import type { DepthLevel, OrderbookSnapshot } from "../types";

const LEVELS = 8;

type Props = {
  symbol: string | undefined;
};

/**
 * Resting bids and asks for one market, from GET /orderbook/{symbol}.
 *
 * The book lives in the matching engine's memory, not in Postgres, and this is
 * a snapshot of it — accurate at the instant the worker read it. It is polled
 * rather than streamed, so what is on screen is always slightly behind; that is
 * a property worth stating in the UI rather than hiding.
 *
 * Asks are shown highest-first so the two sides meet in the middle at the
 * spread, which is how a book is conventionally read even though the API
 * returns both best-first.
 */
export const OrderBook: React.FC<Props> = ({ symbol }) => {
  const { reference } = useReference();

  const [book, setBook] = useState<OrderbookSnapshot>();
  const [error, setError] = useState<string>();

  const load = useCallback(async () => {
    if (!symbol) return;
    try {
      setBook(await api.getOrderbook(symbol, LEVELS));
      setError(undefined);
    } catch (err) {
      setError(errorMessage(err));
    }
  }, [symbol]);

  usePolling(load, 2500, Boolean(symbol));

  const bids = book?.bids ?? [];
  const asks = book?.asks ?? [];

  // One scale across both sides, so a bar's length is comparable between them
  // rather than each side being normalised to its own maximum.
  const deepest = Math.max(1, ...bids.map((l) => l.quantity), ...asks.map((l) => l.quantity));

  const spread =
    bids.length && asks.length ? asks[0].price - bids[0].price : undefined;

  const row = (level: DepthLevel, side: "bid" | "ask") => (
    <tr key={`${side}-${level.price}`}>
      <td style={{ position: "relative" }}>
        <div
          aria-hidden
          style={{
            position: "absolute",
            inset: "2px auto 2px 0",
            width: `${(level.quantity / deepest) * 100}%`,
            background: side === "bid" ? "var(--success)" : "var(--danger)",
            opacity: 0.14,
            borderRadius: 2
          }}
        />
        <span style={{ position: "relative", color: side === "bid" ? "var(--success)" : "var(--danger)" }}>
          {symbol ? formatPrice(reference, symbol, level.price) : level.price}
        </span>
      </td>
      <td style={{ textAlign: "right" }}>
        {symbol ? formatQuantity(reference, symbol, level.quantity) : level.quantity}
      </td>
      <td style={{ textAlign: "right" }} className="muted">
        {level.orders}
      </td>
    </tr>
  );

  return (
    <SourcedPanel
      eyebrow="Depth"
      title={symbol ? `Order book — ${symbol}` : "Order book"}
      kind="live"
      endpoint="GET /orderbook/{symbol}"
      note={
        <>
          Resting orders in the matching engine's memory, refreshed every couple of seconds. Volume
          shown is what is actually matchable — cancelled orders are excluded even before the engine
          evicts them.
        </>
      }
    >
      {error && <div className="pill status-danger">{error}</div>}

      {!symbol && <div className="muted">Pick a market to see its book.</div>}

      {symbol && (
        <table className="table">
          <thead>
            <tr>
              <th>Price</th>
              <th style={{ textAlign: "right" }}>Quantity</th>
              <th style={{ textAlign: "right" }}>Orders</th>
            </tr>
          </thead>
          <tbody>
            {/* Highest ask at the top, descending to the spread. */}
            {[...asks].reverse().map((level) => row(level, "ask"))}

            <tr>
              <td colSpan={3} className="muted" style={{ textAlign: "center", fontSize: 12 }}>
                {spread === undefined
                  ? "one side is empty — no spread"
                  : `spread ${formatPrice(reference, symbol, spread)}`}
              </td>
            </tr>

            {bids.map((level) => row(level, "bid"))}

            {!bids.length && !asks.length && (
              <tr>
                <td colSpan={3} className="muted">
                  Nothing resting on this book.
                </td>
              </tr>
            )}
          </tbody>
        </table>
      )}
    </SourcedPanel>
  );
};
