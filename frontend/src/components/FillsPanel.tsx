import React, { useCallback, useState } from "react";
import { SourcedPanel } from "./DataSource";
import { useAuth } from "../hooks/useAuth";
import { usePolling } from "../hooks/usePolling";
import { useReference } from "../hooks/useReference";
import { ApiError, api, errorMessage } from "../lib/api";
import { formatPrice, formatQuantity } from "../lib/markets";
import type { Fill } from "../types";

const FILLS = 25;

/**
 * The caller's own executions, from GET /trades.
 *
 * This is the detail behind the fill progress on the orders table above: that
 * says how much of an order filled, this says which trades did it and at what
 * price. An order that walked several price levels shows one row per level —
 * which is the case where the two views stop looking like the same information.
 *
 * Rows are keyed on id AND side: a user on both sides of one trade gets two
 * entries sharing a trade id, and React would otherwise drop one.
 */
export const FillsPanel: React.FC = () => {
  const { token, logout } = useAuth();
  const { reference } = useReference();

  const [fills, setFills] = useState<Fill[]>([]);
  const [error, setError] = useState<string>();
  const [loaded, setLoaded] = useState(false);

  const load = useCallback(async () => {
    if (!token) return;
    try {
      const res = await api.getTrades(token, FILLS);
      setFills(res.trades ?? []);
      setError(undefined);
    } catch (err) {
      if (err instanceof ApiError && err.isUnauthorized) {
        logout();
        return;
      }
      setError(errorMessage(err));
    } finally {
      setLoaded(true);
    }
  }, [logout, token]);

  usePolling(load, 5000, Boolean(token));

  return (
    <SourcedPanel
      eyebrow="Executions"
      title="Your fills"
      kind="live"
      endpoint="GET /trades"
      note={
        <>
          One row per execution, newest first. <strong>Taker</strong> means your order crossed the
          spread — a taker buy pays at most its limit and often less, and the difference returns to
          your available balance when the order closes.
        </>
      }
    >
      {error && <div className="pill status-danger">{error}</div>}

      <table className="table">
        <thead>
          <tr>
            <th>Time</th>
            <th>Market</th>
            <th>Order</th>
            <th>Side</th>
            <th style={{ textAlign: "right" }}>Quantity</th>
            <th style={{ textAlign: "right" }}>Price</th>
            <th>Role</th>
          </tr>
        </thead>
        <tbody>
          {fills.map((fill) => (
            <tr key={`${fill.id}-${fill.side}`}>
              <td className="muted">{new Date(fill.executed_at).toLocaleString()}</td>
              <td>{fill.market}</td>
              <td className="muted">#{fill.order_id}</td>
              <td style={{ color: fill.side === "buy" ? "var(--success)" : "var(--danger)" }}>
                {fill.side}
              </td>
              <td style={{ textAlign: "right" }}>
                {formatQuantity(reference, fill.market, fill.quantity)}
              </td>
              <td style={{ textAlign: "right" }}>
                {formatPrice(reference, fill.market, fill.price)}
              </td>
              <td>
                <span className="tag">{fill.taker ? "taker" : "maker"}</span>
              </td>
            </tr>
          ))}

          {loaded && !fills.length && (
            <tr>
              <td colSpan={7} className="muted">
                Nothing has filled yet. An order only executes when it crosses a resting one on the
                other side.
              </td>
            </tr>
          )}
        </tbody>
      </table>
    </SourcedPanel>
  );
};
