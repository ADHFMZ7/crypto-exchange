import { useEffect, useRef, useState } from "react";
import { subscribe, type StreamEvent, type StreamStatus } from "../lib/stream";
import type { MarketTrade, OrderbookSnapshot } from "../types";

export type { StreamStatus } from "../lib/stream";

type Handlers = {
  /** A trade that has settled. Append it. */
  onTrade?: (trade: MarketTrade) => void;
  /** The book as it now stands. Replace what you have — it is not a diff. */
  onDepth?: (book: OrderbookSnapshot) => void;
  /** Fires on every connect, including reconnects. Re-read the REST snapshot. */
  onResync?: () => void;
};

/**
 * Subscribe a component to a market's live feed.
 *
 * Callbacks are held in a ref so a caller can pass inline functions without
 * resubscribing on every render — the connection depends on the symbol alone.
 */
export function useMarketStream(symbol: string | undefined, handlers: Handlers): StreamStatus {
  const [status, setStatus] = useState<StreamStatus>("connecting");

  const latest = useRef(handlers);
  useEffect(() => {
    latest.current = handlers;
  });

  useEffect(() => {
    if (!symbol) return;

    return subscribe(symbol, {
      onStatus: setStatus,
      onResync: () => latest.current.onResync?.(),
      onEvent: (event: StreamEvent) => {
        if (event.type === "trade") latest.current.onTrade?.(event.payload);
        if (event.type === "depth") latest.current.onDepth?.(event.payload);
      }
    });
  }, [symbol]);

  return status;
}
