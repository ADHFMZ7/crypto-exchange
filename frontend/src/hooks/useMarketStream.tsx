import { useEffect, useRef, useState } from "react";
import { API_BASE } from "../lib/api";
import type { MarketTrade } from "../types";

/**
 * The live execution feed.
 *
 * Polling asked "has anything happened?" several times a second and was told no
 * almost every time. This is told once, when something does.
 *
 * A stream only carries what happens after you connect, so it cannot be the
 * whole story: the caller still reads the REST tape for the state before that.
 * `onResync` fires on every connect, including reconnects, because the gap
 * while disconnected is exactly the window the stream cannot fill.
 */

export type StreamStatus = "connecting" | "live" | "offline";

/** Backoff between reconnects, capped. The server is on the same machine in
 *  development, so the first retry is quick and the ceiling is what matters. */
const RETRY_MS = [500, 1000, 2000, 5000, 10_000];

function streamURL(symbol: string): string {
  const base = API_BASE.replace(/^http/, "ws");
  return `${base}/stream?markets=${encodeURIComponent(symbol)}`;
}

export function useMarketStream(
  symbol: string | undefined,
  onTrade: (trade: MarketTrade) => void,
  onResync: () => void
): StreamStatus {
  const [status, setStatus] = useState<StreamStatus>("connecting");

  // Held in refs so a caller can pass inline functions without tearing the
  // connection down and rebuilding it on every render.
  const trade = useRef(onTrade);
  const resync = useRef(onResync);
  useEffect(() => {
    trade.current = onTrade;
    resync.current = onResync;
  }, [onResync, onTrade]);

  useEffect(() => {
    if (!symbol) return;

    let socket: WebSocket | undefined;
    let retry: number | undefined;
    let attempt = 0;
    let abandoned = false;

    const connect = () => {
      if (abandoned) return;
      setStatus((prev) => (prev === "live" ? "connecting" : prev));

      socket = new WebSocket(streamURL(symbol));

      socket.onopen = () => {
        attempt = 0;
        setStatus("live");
        // Whatever happened while disconnected is not on this connection.
        resync.current();
      };

      socket.onmessage = (message) => {
        try {
          const event = JSON.parse(message.data as string);
          if (event?.type === "trade" && event.payload) {
            trade.current(event.payload as MarketTrade);
          }
        } catch {
          // A frame we cannot read is not worth dropping the connection over.
        }
      };

      socket.onclose = () => {
        if (abandoned) return;
        setStatus("offline");
        // The server closes with TryAgainLater when a client has fallen behind,
        // and the browser closes on a dropped network. Both want the same thing.
        const wait = RETRY_MS[Math.min(attempt, RETRY_MS.length - 1)];
        attempt += 1;
        retry = window.setTimeout(connect, wait);
      };

      // onerror is always followed by onclose, which owns the retry.
      socket.onerror = () => setStatus("offline");
    };

    connect();

    return () => {
      abandoned = true;
      window.clearTimeout(retry);
      // Detach the handlers first: closing fires onclose, which would otherwise
      // schedule a reconnect for a component that is going away.
      if (socket) {
        socket.onopen = null;
        socket.onmessage = null;
        socket.onclose = null;
        socket.onerror = null;
        socket.close();
      }
    };
  }, [symbol]);

  return status;
}
