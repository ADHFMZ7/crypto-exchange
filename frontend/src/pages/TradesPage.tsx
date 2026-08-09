import React, { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { SourcedPanel } from "../components/DataSource";
import { IntegrationStatus } from "../components/IntegrationStatus";
import { MarketTable } from "../components/MarketTable";
import { useReference } from "../hooks/useReference";
import { formatOrderLegs } from "../lib/markets";
import { clearOrderLog, readOrderLog } from "../lib/orderLog";
import type { LocalOrder } from "../types";

const TYPE_LABEL: Record<LocalOrder["type"], string> = {
  limit_buy: "buy",
  limit_sell: "sell",
  cancel: "cancel"
};

export const TradesPage: React.FC = () => {
  const [orders, setOrders] = useState<LocalOrder[]>([]);
  const { reference } = useReference();

  useEffect(() => {
    setOrders(readOrderLog());
  }, []);


  return (
    <div className="grid" style={{ gap: 18 }}>
      <SourcedPanel
        eyebrow="Trades"
        title="Orders submitted from this browser"
        kind="local"
        endpoint="awaiting GET /orders"
        note={
          <>
            These are real acknowledgements from <code>POST /trades</code>, saved locally because the
            backend has no endpoint to read orders back. <strong>Fill status is unknown</strong> — an
            order here may have filled, rested, or been cancelled, and clearing site data erases the
            list. Amounts are rendered from the minor units that were sent, so any entry logged
            before this browser switched to cents and satoshis will read far too small — clear the
            log if you see one.
          </>
        }
        actions={
          orders.length > 0 ? (
            <button
              type="button"
              className="ghost-button"
              onClick={() => setOrders(clearOrderLog())}
            >
              Clear log
            </button>
          ) : undefined
        }
      >
        <table className="table" style={{ marginTop: 4 }}>
          <thead>
            <tr>
              <th>Order ID</th>
              <th>Market</th>
              <th>Side</th>
              <th style={{ textAlign: "right" }}>Shares</th>
              <th style={{ textAlign: "right" }}>Price</th>
              <th>Submitted</th>
            </tr>
          </thead>
          <tbody>
            {orders.map((order) => {
              const legs = formatOrderLegs(reference, order);
              return (
                <tr key={`${order.order_id}-${order.submittedAt}`}>
                  <td>{order.order_id}</td>
                  <td>{order.market}</td>
                  <td
                    style={{
                      color:
                        order.type === "limit_buy"
                          ? "var(--success)"
                          : order.type === "limit_sell"
                            ? "var(--danger)"
                            : "var(--muted-text)"
                    }}
                  >
                    {TYPE_LABEL[order.type]}
                  </td>
                  <td style={{ textAlign: "right" }}>{legs.shares}</td>
                  <td style={{ textAlign: "right" }}>{legs.price}</td>
                  <td className="muted">{new Date(order.submittedAt).toLocaleString()}</td>
                </tr>
              );
            })}
            {orders.length === 0 && (
              <tr>
                <td colSpan={6} className="muted">
                  Nothing submitted yet. <Link to="/trades/new">Place an order →</Link>
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </SourcedPanel>

      <MarketTable />

      <IntegrationStatus />
    </div>
  );
};
