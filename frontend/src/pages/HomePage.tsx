import React, { useEffect, useMemo, useState } from "react";
import { Link } from "react-router-dom";
import { useAuth } from "../hooks/useAuth";
import type { MarketTicker, Trade, WalletBalance } from "../types";

const mockBalances: WalletBalance[] = [
  { currency: "USD", amount: 10000 },
  { currency: "BTC", amount: 0.42 },
  { currency: "ETH", amount: 3.5 }
];

const sampleTrades: Trade[] = [
  { id: "1", market: "BTC-USD", side: "buy", quantity: 0.1, price: 45000, status: "open", placedAt: "2024-01-01T12:00:00Z" },
  { id: "2", market: "ETH-USD", side: "sell", quantity: 1.5, price: 3200, status: "open", placedAt: "2024-01-02T09:35:00Z" },
  { id: "3", market: "SOL-USD", side: "buy", quantity: 50, price: 110, status: "filled", placedAt: "2024-01-03T15:12:00Z" }
];

const initialMarkets: MarketTicker[] = [
  { symbol: "BTC-USD", price: 45210, change: 2.1, volume: 2150 },
  { symbol: "ETH-USD", price: 3230, change: -0.8, volume: 8891 },
  { symbol: "SOL-USD", price: 112, change: 1.6, volume: 12540 },
  { symbol: "DOGE-USD", price: 0.088, change: 5.2, volume: 102001 }
];

type PricePoint = { t: number; price: number };
const chartSymbols = ["BTC-USD", "ETH-USD", "SOL-USD"];

export const HomePage: React.FC = () => {
  const { user } = useAuth();
  const [trades] = useState<Trade[]>(sampleTrades);
  const [markets, setMarkets] = useState<MarketTicker[]>(initialMarkets);
  const [selectedSymbol, setSelectedSymbol] = useState<string>(chartSymbols[0]);
  const [series, setSeries] = useState<Record<string, PricePoint[]>>(() => {
    const now = Date.now();
    const start = (price: number) =>
      Array.from({ length: 12 }, (_, i) => ({
        t: now - (11 - i) * 60_000,
        price: Number((price * (1 + (Math.random() - 0.5) * 0.01)).toFixed(2))
      }));
    return {
      "BTC-USD": start(45000),
      "ETH-USD": start(3200),
      "SOL-USD": start(110)
    };
  });

  const openTrades = useMemo(() => trades.filter((t) => t.status === "open"), [trades]);
  const netOpenExposure = useMemo(
    () =>
      openTrades.reduce((sum, trade) => {
        const value = trade.price * trade.quantity;
        return sum + (trade.side === "buy" ? value : -value);
      }, 0),
    [openTrades]
  );

  useEffect(() => {
    const id = setInterval(() => {
      // update ticker cards
      setMarkets((prev) =>
        prev.map((m) => {
          const drift = (Math.random() - 0.5) * (m.price * 0.0015);
          const nextPrice = Math.max(m.price + drift, 0.0001);
          const nextChange = ((nextPrice - m.price) / m.price) * 100 + m.change;
          return { ...m, price: Number(nextPrice.toFixed(2)), change: Number(nextChange.toFixed(2)) };
        })
      );

      // append a price point to each chart symbol to simulate executed trades
      setSeries((prev) => {
        const next: typeof prev = {};
        Object.entries(prev).forEach(([symbol, points]) => {
          const last = points[points.length - 1];
          const drift = (Math.random() - 0.5) * (last.price * 0.002);
          const price = Math.max(last.price + drift, 0.0001);
          const updated = [...points.slice(-30), { t: Date.now(), price: Number(price.toFixed(2)) }];
          next[symbol] = updated;
        });
        return next;
      });
    }, 2500);
    return () => clearInterval(id);
  }, []);

  const currentSeries = series[selectedSymbol] ?? [];
  const chartMetrics = useMemo(() => {
    if (!currentSeries.length) return { min: 0, max: 0, path: "" };
    const min = Math.min(...currentSeries.map((p) => p.price));
    const max = Math.max(...currentSeries.map((p) => p.price));
    const padding = (max - min || max || 1) * 0.1;
    const low = min - padding;
    const high = max + padding;
    const xSpan = Math.max(...currentSeries.map((p) => p.t)) - Math.min(...currentSeries.map((p) => p.t)) || 1;
    const mapPoint = (p: PricePoint) => {
      const x = ((p.t - currentSeries[0].t) / xSpan) * 100;
      const y = 100 - ((p.price - low) / (high - low)) * 100;
      return { x, y };
    };
    const coords = currentSeries.map(mapPoint);
    const path = coords.map((c, i) => `${i === 0 ? "M" : "L"}${c.x},${c.y}`).join(" ");
    return { min, max, path, last: currentSeries[currentSeries.length - 1] };
  }, [currentSeries]);

  return (
    <div className="grid" style={{ gap: 18 }}>
      <div className="panel">
        <div className="headline">
          <div>
            <div className="tag">Market moves</div>
            <h2 style={{ margin: "4px 0" }}>{selectedSymbol}</h2>
            <div className="muted">Simulated executed trades over time — price in USD on the Y axis.</div>
          </div>
          <div className="inline-actions">
            <div className="pill">
              <div className="muted">Latest</div>
              <strong>{chartMetrics.last?.price ?? "-"} USD</strong>
            </div>
            <div className="pill">
              <div className="muted">Range</div>
              <strong>
                {chartMetrics.min.toLocaleString(undefined, { maximumFractionDigits: 2 })} –{" "}
                {chartMetrics.max.toLocaleString(undefined, { maximumFractionDigits: 2 })} USD
              </strong>
            </div>
          </div>
        </div>

        <div style={{ width: "100%", height: 260, position: "relative" }}>
          <svg viewBox="0 0 100 100" preserveAspectRatio="none" style={{ width: "100%", height: "100%" }}>
            <defs>
              <linearGradient id="areaFill" x1="0" x2="0" y1="0" y2="1">
                <stop offset="0%" stopColor="var(--accent)" stopOpacity="0.25" />
                <stop offset="100%" stopColor="var(--accent)" stopOpacity="0" />
              </linearGradient>
            </defs>
            {chartMetrics.path && (
              <>
                <path
                  d={`${chartMetrics.path} L100,100 L0,100 Z`}
                  fill="url(#areaFill)"
                  stroke="none"
                  vectorEffect="non-scaling-stroke"
                />
                <path
                  d={chartMetrics.path}
                  fill="none"
                  stroke="var(--accent)"
                  strokeWidth={1.5}
                  vectorEffect="non-scaling-stroke"
                />
              </>
            )}
          </svg>
          <div style={{ position: "absolute", bottom: 8, right: 12 }}>
            <label className="inline-actions" style={{ gap: 8, alignItems: "center" }}>
              <span className="muted">Currency</span>
              <select value={selectedSymbol} onChange={(e) => setSelectedSymbol(e.target.value)} style={{ width: 140 }}>
                {chartSymbols.map((s) => (
                  <option key={s} value={s}>
                    {s}
                  </option>
                ))}
              </select>
            </label>
          </div>
        </div>
      </div>

      <div className="panel">
        <div className="headline">
          <div>
            <div className="tag">Account</div>
            <h2 style={{ margin: "4px 0" }}>{user ? `Hello, ${user.fullname}` : "Welcome"}</h2>
            <div className="muted">{user?.email}</div>
          </div>
          <div className="inline-actions">
            <Link to="/wallet">
              <button type="button" className="ghost-button">
                Wallet
              </button>
            </Link>
            <Link to="/trades/new">
              <button type="button">New Trade</button>
            </Link>
          </div>
        </div>
        <p className="muted">
          Market snapshot, active trades, and wallet balances. Trades and market feeds are mocked until backend endpoints exist.
        </p>
      </div>

      <div className="panel">
        <div className="headline">
          <h3 style={{ margin: 0 }}>Wallet</h3>
          <div className="tag">Prototype</div>
        </div>
        <table className="table">
          <thead>
            <tr>
              <th>Currency</th>
              <th>Balance</th>
            </tr>
          </thead>
          <tbody>
            {mockBalances.map((balance) => (
              <tr key={balance.currency}>
                <td>{balance.currency}</td>
                <td>{balance.amount}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      <div className="panel">
        <div className="headline">
          <h3 style={{ margin: 0 }}>Open trades</h3>
          <div className="pill">Net exposure: {netOpenExposure.toLocaleString(undefined, { maximumFractionDigits: 2 })} USD</div>
        </div>
        <table className="table">
          <thead>
            <tr>
              <th>Market</th>
              <th>Side</th>
              <th>Quantity</th>
              <th>Price</th>
              <th>Placed</th>
            </tr>
          </thead>
          <tbody>
            {openTrades.map((trade) => (
              <tr key={trade.id}>
                <td>{trade.market}</td>
                <td style={{ color: trade.side === "buy" ? "var(--success)" : "var(--danger)" }}>{trade.side}</td>
                <td>{trade.quantity}</td>
                <td>{trade.price}</td>
                <td>{new Date(trade.placedAt).toLocaleString()}</td>
              </tr>
            ))}
            {openTrades.length === 0 && (
              <tr>
                <td colSpan={5} className="muted">
                  No open trades. Place a new order to see it here.
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </div>

      <div className="panel">
        <div className="headline">
          <h3 style={{ margin: 0 }}>Markets (live mock)</h3>
          <Link to="/trades">
            <button type="button" className="ghost-button">
              View all
            </button>
          </Link>
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
