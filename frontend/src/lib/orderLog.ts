import type { LocalOrder } from "../types";

/**
 * POST /trades returns an ack and nothing else — there is no GET /orders, so a
 * submitted order would otherwise vanish on navigation. We keep the acks here so
 * the Trades page can show what this browser actually sent.
 *
 * This is a client-side receipt book, NOT order state. It cannot know whether an
 * order filled, and it is wiped by clearing site data. Everything rendered from
 * it is labelled "Browser only".
 *
 * Retired by: GET /orders — at which point this file and its call sites go away.
 */

const STORAGE_KEY = "crypto-exchange-order-log";
const MAX_ENTRIES = 100;

export function readOrderLog(): LocalOrder[] {
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    if (!raw) return [];
    const parsed: unknown = JSON.parse(raw);
    if (!Array.isArray(parsed)) return [];
    return parsed as LocalOrder[];
  } catch {
    // Corrupt or unparseable payload should not take the page down.
    return [];
  }
}

export function appendOrder(order: LocalOrder): LocalOrder[] {
  const next = [order, ...readOrderLog()].slice(0, MAX_ENTRIES);
  writeOrderLog(next);
  return next;
}

export function clearOrderLog(): LocalOrder[] {
  writeOrderLog([]);
  return [];
}

function writeOrderLog(orders: LocalOrder[]): void {
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(orders));
  } catch {
    // Quota exceeded or storage disabled — the log is best-effort.
  }
}
