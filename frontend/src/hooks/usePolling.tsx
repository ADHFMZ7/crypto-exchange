import { useEffect, useRef } from "react";

/**
 * Re-runs an effect on an interval, and once immediately.
 *
 * Settlement happens after the 202 that accepted an order, so orders and
 * balances both change with no user action. Without polling the page is a
 * snapshot of whenever it mounted, and a fill looks like nothing happened.
 *
 * The callback is held in a ref so a caller can pass an inline function without
 * restarting the timer on every render — the interval depends on `intervalMs`
 * and `enabled` alone.
 *
 * Polling pauses while the tab is hidden. A background tab that keeps fetching
 * is load with nobody watching, and browsers throttle its timers anyway, so the
 * cadence would be a lie.
 */
export function usePolling(callback: () => void, intervalMs: number, enabled = true): void {
  const saved = useRef(callback);

  useEffect(() => {
    saved.current = callback;
  }, [callback]);

  useEffect(() => {
    if (!enabled) return;

    const run = () => {
      if (document.visibilityState === "visible") saved.current();
    };

    run();
    const timer = window.setInterval(run, intervalMs);

    // Fire on the way back from a hidden tab, so returning to the page shows
    // current data rather than whatever was true when it was backgrounded.
    document.addEventListener("visibilitychange", run);

    return () => {
      window.clearInterval(timer);
      document.removeEventListener("visibilitychange", run);
    };
  }, [enabled, intervalMs]);
}
