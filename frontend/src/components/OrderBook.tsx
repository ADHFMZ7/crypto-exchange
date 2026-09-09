import React, { useCallback, useState } from "react";
import { SourcedPanel } from "./DataSource";
import { usePolling } from "../hooks/usePolling";
import { useReference } from "../hooks/useReference";
import { api, errorMessage } from "../lib/api";
import { formatPrice, formatQuantity } from "../lib/markets";
import type { DepthLevel, OrderbookSnapshot } from "../types";

// Five a side, plus the spread row, is what fits one tile without clipping.
// Deeper and the far bids fall below the fold — which hides the half a buyer
// is actually resting against, the opposite of what the panel is for.
const LEVELS = 5;

type Props = {
  symbol: string | undefined;
  /** Lets the ticket seed its price from the touch without fetching it again. */
  onSnapshot?: (book: OrderbookSnapshot) => void;
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
export const OrderBook: React.FC<Props> = ({ symbol, onSnapshot }) => {
  const { reference } = useReference();

  const [book, setBook] = useState<OrderbookSnapshot>();
  const [error, setError] = useState<string>();

  const load = useCallback(async () => {
    if (!symbol) return;
    try {
      const snapshot = await api.getOrderbook(symbol, LEVELS);
      setBook(snapshot);
      onSnapshot?.(snapshot);
      setError(undefined);
    } catch (err) {
      setError(errorMessage(err));
    }
  }, [onSnapshot, symbol]);

  usePolling(load, 2500, Boolean(symbol));

  const bids = book?.bids ?? [];
  const asks = book?.asks ?? [];

  // One scale across both sides, so a bar's length is comparable between them
  // rather than each side being normalised to its own maximum.
  const deepest = Math.max(1, ...bids.map((l) => l.quantity), ...asks.map((l) => l.quantity));

  const spread =
    bids.length && asks.length ? asks[0].price - bids[0].price : undefined;

  const row = (level: DepthLevel, side: "bid" | "ask") => {
    const share = Math.max(2, Math.round((level.quantity / deepest) * 100));
    const tone = side === "bid" ? "var(--success)" : "var(--danger)";

    return (
      <tr
        key={`${side}-${level.price}`}
        // A gradient stop rather than an absolutely-positioned bar, so the
        // depth reads across the whole row instead of inside one cell.
        style={{
          backgroundImage: `linear-gradient(to left, color-mix(in srgb, ${tone} 16%, transparent) ${share}%, transparent ${share}%)`
        }}
      >
        <td style={{ color: tone }}>
          {symbol ? formatPrice(reference, symbol, level.price) : level.price}
        </td>
        <td style={{ textAlign: "right" }}>
          {symbol ? formatQuantity(reference, symbol, level.quantity) : level.quantity}
        </td>
        <td style={{ textAlign: "right" }} className="muted">
          {level.orders}
        </td>
      </tr>
    );
  };

  return (
    <SourcedPanel
      eyebrow="Depth"
      title="Order book"
      kind="live"
      endpoint="GET /orderbook/{symbol}"
      fill
      note={<>Resting in the engine's memory. Cancelled orders are already excluded.</>}
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
