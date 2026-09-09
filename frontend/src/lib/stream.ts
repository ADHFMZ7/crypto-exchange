import { API_BASE } from "./api";
import type { MarketTrade, OrderbookSnapshot } from "../types";

/**
 * One live connection per market, shared by everything watching it.
 *
 * The trade screen has a tape and a book side by side, both wanting the same
 * feed. A connection per component would open two sockets to one endpoint for
 * one market and receive every message twice, so connections are keyed by
 * symbol and reference counted: the first subscriber opens it, the last one to
 * leave closes it.
 *
 * Nothing here is authoritative. The stream carries what happens after it
 * connects, so a subscriber still reads REST for the state before that, and
 * again on every reconnect — the gap while disconnected is the one window the
 * feed cannot fill, which is what `resync` announces.
 */

export type StreamStatus = "connecting" | "live" | "offline";

export type StreamEvent =
  | { type: "trade"; market: string; payload: MarketTrade }
  | { type: "depth"; market: string; payload: OrderbookSnapshot };

export type StreamHandlers = {
  onEvent?: (event: StreamEvent) => void;
  /** Fires on every connect, including reconnects. Re-read the REST snapshot. */
  onResync?: () => void;
  onStatus?: (status: StreamStatus) => void;
};

/** Backoff between reconnects, capped. */
const RETRY_MS = [500, 1000, 2000, 5000, 10_000];

type Connection = {
  socket?: WebSocket;
  handlers: Set<StreamHandlers>;
  status: StreamStatus;
  attempt: number;
  retry?: number;
  closed: boolean;
};

const connections = new Map<string, Connection>();

function url(symbol: string): string {
  return `${API_BASE.replace(/^http/, "ws")}/stream?markets=${encodeURIComponent(symbol)}`;
}

function announce(connection: Connection, status: StreamStatus) {
  connection.status = status;
  connection.handlers.forEach((h) => h.onStatus?.(status));
}

function open(symbol: string, connection: Connection) {
  if (connection.closed) return;

  const socket = new WebSocket(url(symbol));
  connection.socket = socket;

  socket.onopen = () => {
    connection.attempt = 0;
    announce(connection, "live");
    connection.handlers.forEach((h) => h.onResync?.());
  };

  socket.onmessage = (message) => {
    let event: StreamEvent;
    try {
      event = JSON.parse(message.data as string) as StreamEvent;
    } catch {
      return; // A frame we cannot read is not worth dropping the connection over.
    }
    if (event?.type) connection.handlers.forEach((h) => h.onEvent?.(event));
  };

  socket.onclose = () => {
    if (connection.closed) return;
    announce(connection, "offline");

    // The server closes with TryAgainLater when a client has fallen behind, and
    // the browser closes on a dropped network. Both want the same thing.
    const wait = RETRY_MS[Math.min(connection.attempt, RETRY_MS.length - 1)];
    connection.attempt += 1;
    connection.retry = window.setTimeout(() => open(symbol, connection), wait);
  };

  // onerror is always followed by onclose, which owns the retry.
  socket.onerror = () => announce(connection, "offline");
}

/**
 * Attach to a market's feed. Returns the detach function.
 *
 * The handler object is held by identity, so a caller passing a fresh object on
 * every render would subscribe repeatedly — useMarketStream keeps one in a ref
 * for exactly that reason.
 */
export function subscribe(symbol: string, handlers: StreamHandlers): () => void {
  let connection = connections.get(symbol);

  if (!connection) {
    connection = { handlers: new Set(), status: "connecting", attempt: 0, closed: false };
    connections.set(symbol, connection);
    open(symbol, connection);
  }

  connection.handlers.add(handlers);
  handlers.onStatus?.(connection.status);

  return () => {
    const current = connections.get(symbol);
    if (!current) return;

    current.handlers.delete(handlers);
    if (current.handlers.size > 0) return;

    // The last watcher left. Tear the socket down rather than holding it open
    // for a page nobody is on.
    current.closed = true;
    window.clearTimeout(current.retry);
    if (current.socket) {
      current.socket.onopen = null;
      current.socket.onmessage = null;
      current.socket.onclose = null;
      current.socket.onerror = null;
      current.socket.close();
    }
    connections.delete(symbol);
  };
}
