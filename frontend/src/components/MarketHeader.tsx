import React, { useCallback, useState } from "react";
import { useDeveloperMode } from "../hooks/useDeveloperMode";
import { usePolling } from "../hooks/usePolling";
import { useReference } from "../hooks/useReference";
import { api, errorMessage } from "../lib/api";
import { formatPrice, formatQuantity, percentChange } from "../lib/markets";
import type { Ticker } from "../types";

type Props = {
  symbol: string | undefined;
  /** Rendered on the right of the strip — the market picker, when there is one. */
  children?: React.ReactNode;
};

/**
 * The market strip above the ticket: what this market last traded at, and how
 * it has moved over the trailing window.
 *
 * It sits here rather than in a panel of its own because it is context for the
 * price field directly below it. With no external feed, the last trade is the
 * closest thing to a reference price the exchange has.
 */
export const MarketHeader: React.FC<Props> = ({ symbol, children }) => {
  const { reference } = useReference();
  const { developer } = useDeveloperMode();

  const [tickers, setTickers] = useState<Ticker[]>([]);
  const [error, setError] = useState<string>();

  const load = useCallback(async () => {
    try {
      const res = await api.getTickers();
      setTickers(res.tickers ?? []);
      setError(undefined);
    } catch (err) {
      setError(errorMessage(err));
    }
  }, []);

  usePolling(load, 8000);

  const ticker = tickers.find((t) => t.market === symbol);
  const percent = ticker ? percentChange(ticker) : undefined;
  const up = (ticker?.change ?? 0) >= 0;

  const stat = (label: string, value: React.ReactNode) => (
    <div>
      <div className="market-stat-label">{label}</div>
      <div className="market-stat-value">{value}</div>
    </div>
  );

  return (
    // The strip is a panel built by hand rather than a SourcedPanel, so it has
    // to honour developer mode itself.
    <div className={`panel${developer ? " panel-live" : ""} market-header`}>
      <div className="market-identity">
        <span className="brand" style={{ fontSize: 20 }}>{symbol ?? "—"}</span>
        {developer && (
          <span className="tag source-badge source-live">
            <span aria-hidden="true">●</span>
            Live
            <code className="source-endpoint">GET /markets/tickers</code>
          </span>
        )}
      </div>

      {ticker?.has_traded ? (
        <>
          <div className="market-last">
            <span
              className="market-last-price"
              style={{ color: up ? "var(--success)" : "var(--danger)" }}
            >
              {formatPrice(reference, ticker.market, ticker.last_price)}
            </span>
            <span
              className="market-change"
              style={{ color: up ? "var(--success)" : "var(--danger)" }}
            >
              {up ? "+" : ""}
              {formatPrice(reference, ticker.market, ticker.change)}
              {percent !== undefined && ` (${percent >= 0 ? "+" : ""}${percent.toFixed(2)}%)`}
            </span>
          </div>

          <div className="market-stats">
            {stat(`${ticker.window_hours}h high`, formatPrice(reference, ticker.market, ticker.high))}
            {stat(`${ticker.window_hours}h low`, formatPrice(reference, ticker.market, ticker.low))}
            {stat(
              `${ticker.window_hours}h volume`,
              formatQuantity(reference, ticker.market, ticker.base_volume)
            )}
            {stat("Trades", ticker.trade_count.toLocaleString())}
          </div>
        </>
      ) : (
        <div className="muted market-last">
          {error ?? "No trades yet — nothing moves here until an order crosses."}
        </div>
      )}

      {children && <div className="market-header-actions">{children}</div>}
    </div>
  );
};
