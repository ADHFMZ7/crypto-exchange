import React, { useCallback, useState } from "react";
import { SourcedPanel } from "./DataSource";
import { StreamBadge } from "./StreamBadge";
import { useMarketStream } from "../hooks/useMarketStream";
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

  /*
   * Depth arrives pushed rather than asked for.
   *
   * A book snapshot replaces what is on screen rather than being merged into
   * it: the server sends the whole book, not a diff, so there is no sequence
   * number to track and no way for the client's copy to drift from the
   * engine's. That is worth the extra bytes at this size.
   *
   * The engine announces on a timer while the book is changing, so a burst of
   * orders costs a few snapshots rather than one per order.
   */
  const status = useMarketStream(symbol, {
    // The feed broadcasts a generous depth because one message serves every
    // client, and each of them wants a different amount. Trimming here is what
    // keeps the streamed book identical to the one REST returns.
    onDepth: (book) =>
      setBook({
        ...book,
        bids: book.bids.slice(0, LEVELS),
        asks: book.asks.slice(0, LEVELS)
      }),
    onResync: load
  });

  // Only while the feed is down. A book that has silently stopped updating
  // looks exactly like a quiet market, and acting on a stale one loses money.
  usePolling(load, 3000, Boolean(symbol) && status !== "live");

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
      actions={<StreamBadge status={status} />}
      note={<>Offers waiting to be traded against. Sellers above, buyers below.</>}
      devNote={
        <>
          Read from the matching engine's memory, not from Postgres — the worker that owns each book
          announces it, so a snapshot is never a half-applied match. Cancelled orders are excluded
          before the engine evicts them.
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
