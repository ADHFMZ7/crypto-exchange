import React, { useEffect, useMemo, useState } from "react";
import type { MarketTicker, Trade } from "../types";

const initialTrades: Trade[] = [
  { id: "1", market: "BTC-USD", side: "buy", quantity: 0.1, price: 45000, status: "open", placedAt: "2024-01-01T12:00:00Z" },
  { id: "2", market: "ETH-USD", side: "sell", quantity: 1.5, price: 3200, status: "filled", placedAt: "2024-01-02T09:35:00Z" },
  { id: "3", market: "SOL-USD", side: "buy", quantity: 50, price: 110, status: "cancelled", placedAt: "2024-01-03T15:12:00Z" }
];

const initialMarkets: MarketTicker[] = [
  { symbol: "BTC-USD", price: 45210, change: 2.1, volume: 2150 },
  { symbol: "ETH-USD", price: 3230, change: -0.8, volume: 8891 },
  { symbol: "SOL-USD", price: 112, change: 1.6, volume: 12540 },
  { symbol: "DOGE-USD", price: 0.088, change: 5.2, volume: 102001 }
];

const statusClass: Record<Trade["status"], string> = {
  open: "tag",
  filled: "tag status-success",
  cancelled: "tag status-danger",
  settled: "tag status-success"
};

export const TradesPage: React.FC = () => {
  const [trades, setTrades] = useState<Trade[]>(initialTrades);
  const [markets, setMarkets] = useState<MarketTicker[]>(initialMarkets);

  useEffect(() => {
    const id = setInterval(() => {
      setMarkets((prev) =>
        prev.map((m) => {
          const drift = (Math.random() - 0.5) * (m.price * 0.0015);
          const nextPrice = Math.max(m.price + drift, 0.0001);
          const nextChange = ((nextPrice - m.price) / m.price) * 100 + m.change;
          return { ...m, price: Number(nextPrice.toFixed(2)), change: Number(nextChange.toFixed(2)) };
        })
      );
    }, 2500);
    return () => clearInterval(id);
  }, []);

  const totalExposure = useMemo(
    () =>
      trades.reduce((sum, trade) => {
        const value = trade.price * trade.quantity;
        return sum + (trade.side === "buy" ? value : -value);
      }, 0),
    [trades]
  );

  const onClearLocalTrades = () => setTrades([]);

  return (
    <div className="grid" style={{ gap: 18 }}>
      <div className="panel">
        <div className="headline">
          <div>
            <div className="tag">Trades</div>
            <h2 style={{ margin: "4px 0" }}>Activity</h2>
            <div className="muted">Static sample data. Connect to the backend once orders are available.</div>
          </div>
          <button type="button" onClick={onClearLocalTrades} style={{ background: "rgba(255,255,255,0.08)", color: "var(--text)" }}>
            Clear local list
          </button>
        </div>

        <div className="pill">Net exposure (demo): {totalExposure.toLocaleString(undefined, { maximumFractionDigits: 2 })} USD</div>

        <table className="table" style={{ marginTop: 12 }}>
          <thead>
            <tr>
              <th>Market</th>
              <th>Side</th>
              <th>Quantity</th>
              <th>Price</th>
              <th>Status</th>
              <th>Placed</th>
            </tr>
          </thead>
          <tbody>
            {trades.map((trade) => (
              <tr key={trade.id}>
                <td>{trade.market}</td>
                <td style={{ color: trade.side === "buy" ? "var(--success)" : "var(--danger)" }}>{trade.side}</td>
                <td>{trade.quantity}</td>
                <td>{trade.price}</td>
                <td>
                  <span className={statusClass[trade.status]}>{trade.status}</span>
                </td>
                <td>{new Date(trade.placedAt).toLocaleString()}</td>
              </tr>
            ))}
            {trades.length === 0 && (
              <tr>
                <td colSpan={6} className="muted">
                  No trades yet. Use the New Trade page to simulate an order.
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </div>

      <div className="panel">
        <div className="headline">
          <div>
            <div className="tag">Markets</div>
            <h3 style={{ margin: 0 }}>Live mock data</h3>
          </div>
          <button
            type="button"
            onClick={() => setMarkets(initialMarkets)}
            style={{ background: "rgba(255,255,255,0.08)", color: "var(--text)" }}
          >
            Reset
          </button>
        </div>

        <table className="table">
          <thead>
            <tr>
              <th>Symbol</th>
              <th>Price</th>
              <th>Change</th>
              <th>Volume</th>
            </tr>
          </thead>
          <tbody>
            {markets.map((m) => (
              <tr key={m.symbol}>
                <td>{m.symbol}</td>
                <td>{m.price}</td>
                <td style={{ color: m.change >= 0 ? "var(--success)" : "var(--danger)" }}>
                  {m.change >= 0 ? "+" : ""}
                  {m.change}%
                </td>
                <td>{m.volume.toLocaleString()}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
};
