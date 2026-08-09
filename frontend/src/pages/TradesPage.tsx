import React, { useCallback, useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { SourcedPanel } from "../components/DataSource";
import { IntegrationStatus } from "../components/IntegrationStatus";
import { MarketTable } from "../components/MarketTable";
import { useAuth } from "../hooks/useAuth";
import { useReference } from "../hooks/useReference";
import { ApiError, api, errorMessage } from "../lib/api";
import { fillFraction, formatOrderLegs } from "../lib/markets";
import type { Order, OrderStatus } from "../types";

const STATUS_CLASS: Record<OrderStatus, string> = {
  open: "tag",
  partially_filled: "tag",
  filled: "tag status-success",
  cancelled: "tag status-danger"
};

const STATUS_LABEL: Record<OrderStatus, string> = {
  open: "open",
  partially_filled: "partial",
  filled: "filled",
  cancelled: "cancelled"
};

export const TradesPage: React.FC = () => {
  const { token, logout } = useAuth();
  const { reference } = useReference();

  const [orders, setOrders] = useState<Order[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string>();

  const loadOrders = useCallback(async () => {
    if (!token) return;
    setLoading(true);
    setError(undefined);
    try {
      const res = await api.getOrders(token);
      setOrders(res.orders ?? []);
    } catch (err) {
      if (err instanceof ApiError && err.isUnauthorized) {
        logout();
        return;
      }
      setError(errorMessage(err));
    } finally {
      setLoading(false);
    }
  }, [logout, token]);

  useEffect(() => {
    loadOrders();
  }, [loadOrders]);

  return (
    <div className="grid" style={{ gap: 18 }}>
      <SourcedPanel
        eyebrow="Orders"
        title="Your orders"
        kind="live"
        endpoint="GET /orders"
        note={
          <>
            Read from the database, newest first — not from this browser. Fill progress comes from{" "}
            <code>filled_quantity</code>, which stays at zero until the matching engine reports
            executions, so everything currently rests as <strong>open</strong>. That is accurate
            rather than unknown: nothing can fill yet.
          </>
        }
        actions={
          <button type="button" className="ghost-button" onClick={loadOrders} disabled={loading}>
            {loading ? "Refreshing…" : "Refresh"}
          </button>
        }
      >
        {error && <div className="pill status-danger">{error}</div>}

        {!error && (
          <table className="table" style={{ marginTop: 4 }}>
            <thead>
              <tr>
                <th>ID</th>
                <th>Market</th>
                <th>Side</th>
                <th style={{ textAlign: "right" }}>Quantity</th>
                <th style={{ textAlign: "right" }}>Filled</th>
                <th style={{ textAlign: "right" }}>Limit price</th>
                <th>Status</th>
                <th>Placed</th>
              </tr>
            </thead>
            <tbody>
              {orders.map((order) => {
                const legs = formatOrderLegs(reference, order);
                const filled = fillFraction(order);

                return (
                  <tr key={order.id}>
                    <td>{order.id}</td>
                    <td>{order.market}</td>
                    <td
                      style={{
                        color: order.side === "buy" ? "var(--success)" : "var(--danger)"
                      }}
                    >
                      {order.side}
                    </td>
                    <td style={{ textAlign: "right" }}>{legs.quantity}</td>
                    <td style={{ textAlign: "right" }} className={filled ? undefined : "muted"}>
                      {legs.filled}
                      <span className="muted"> ({Math.round(filled * 100)}%)</span>
                    </td>
                    <td style={{ textAlign: "right" }}>{legs.price}</td>
                    <td>
                      <span className={STATUS_CLASS[order.status] ?? "tag"}>
                        {STATUS_LABEL[order.status] ?? order.status}
                      </span>
                    </td>
                    <td className="muted">{new Date(order.created_at).toLocaleString()}</td>
                  </tr>
                );
              })}

              {orders.length === 0 && !loading && (
                <tr>
                  <td colSpan={8} className="muted">
                    No orders yet. <Link to="/trades/new">Place one →</Link>
                  </td>
                </tr>
              )}
              {loading && orders.length === 0 && (
                <tr>
                  <td colSpan={8} className="muted">
                    Loading orders…
                  </td>
                </tr>
              )}
            </tbody>
          </table>
        )}
      </SourcedPanel>

      <MarketTable />

      <IntegrationStatus />
    </div>
  );
};
