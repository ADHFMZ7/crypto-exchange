import React, { useEffect, useMemo, useState } from "react";
import { Link } from "react-router-dom";
import { SourcedPanel } from "../components/DataSource";
import { MarketTable } from "../components/MarketTable";
import { useAuth } from "../hooks/useAuth";
import { useReference } from "../hooks/useReference";
import { ApiError, api, errorMessage } from "../lib/api";
import { toAmount } from "../lib/decimal";
import { formatBalance, formatOrderLegs } from "../lib/markets";
import { readOrderLog } from "../lib/orderLog";
import {
  MOCK_CHART_SYMBOLS,
  MOCK_TICK_MS,
  driftSeries,
  seedSeries,
  type PricePoint
} from "../mocks";
import type { LocalOrder, WalletBalance } from "../types";

export const HomePage: React.FC = () => {
  const { user, token, logout } = useAuth();
  const { reference } = useReference();

  const [balances, setBalances] = useState<WalletBalance[]>([]);
  const [walletError, setWalletError] = useState<string>();
  const [walletLoading, setWalletLoading] = useState(false);

  const [orders, setOrders] = useState<LocalOrder[]>([]);
  const [series, setSeries] = useState<Record<string, PricePoint[]>>(seedSeries);
  const [selectedSymbol, setSelectedSymbol] = useState<string>(MOCK_CHART_SYMBOLS[0]);

  useEffect(() => {
    setOrders(readOrderLog());
  }, []);

  useEffect(() => {
    if (!token) return;
    let cancelled = false;

    (async () => {
      setWalletLoading(true);
      setWalletError(undefined);
      try {
        const wallet = await api.getWallet(token);
        if (!cancelled) setBalances(wallet.balances ?? []);
      } catch (err) {
        if (cancelled) return;
        if (err instanceof ApiError && err.isUnauthorized) {
          logout();
          return;
        }
        setWalletError(errorMessage(err));
      } finally {
        if (!cancelled) setWalletLoading(false);
      }
    })();

    return () => {
      cancelled = true;
    };
  }, [logout, token]);

  useEffect(() => {
    const id = setInterval(() => setSeries(driftSeries), MOCK_TICK_MS);
    return () => clearInterval(id);
  }, []);

  const currentSeries = series[selectedSymbol] ?? [];
  const chart = useMemo(() => {
    if (!currentSeries.length) return { min: 0, max: 0, path: "", last: undefined };

    const prices = currentSeries.map((p) => p.price);
    const min = Math.min(...prices);
    const max = Math.max(...prices);
    const padding = (max - min || max || 1) * 0.1;
    const low = min - padding;
    const high = max + padding;
    const times = currentSeries.map((p) => p.t);
    const xSpan = Math.max(...times) - Math.min(...times) || 1;

    const path = currentSeries
      .map((p, i) => {
        const x = ((p.t - currentSeries[0].t) / xSpan) * 100;
        const y = 100 - ((p.price - low) / (high - low)) * 100;
        return `${i === 0 ? "M" : "L"}${x},${y}`;
      })
      .join(" ");

    return { min, max, path, last: currentSeries[currentSeries.length - 1] };
  }, [currentSeries]);

  const recentOrders = orders.slice(0, 5);

  return (
    <div className="grid" style={{ gap: 18 }}>
      <SourcedPanel
        eyebrow="Account"
        title={user ? `Hello, ${user.fullname}` : "Welcome"}
        kind="live"
        endpoint="GET /users/me, GET /wallets/me"
        note="Identity and balances read from the database."
        actions={
          <>
            <Link to="/wallet">
              <button type="button" className="ghost-button">
                Wallet
              </button>
            </Link>
            <Link to="/trades/new">
              <button type="button">New Trade</button>
            </Link>
          </>
        }
      >
        <div className="muted" style={{ marginBottom: 12 }}>
          {user?.email}
        </div>

        {walletError && <div className="pill status-danger">{walletError}</div>}

        <div className="inline-actions" style={{ gap: 12 }}>
          {balances.map((balance) => (
            <div className="card" key={balance.id}>
              <div className="muted">{balance.currency} available</div>
              <strong style={{ fontSize: 20 }}>
                {formatBalance(reference, balance.available, balance.currency)}
              </strong>
              {toAmount(balance.locked) > 0n && (
                <div className="muted">
                  {formatBalance(reference, balance.locked, balance.currency)} locked
                </div>
              )}
            </div>
          ))}
          {balances.length === 0 && (
            <div className="muted">{walletLoading ? "Loading balances…" : "No balances yet."}</div>
          )}
        </div>
      </SourcedPanel>

      <SourcedPanel
        eyebrow="Market moves"
        title={selectedSymbol}
        kind="mock"
        endpoint="awaiting GET /markets/{symbol}/candles"
        note="A synthetic random walk, not price history. Nothing here reflects real or executed trades."
        actions={
          <div className="pill">
            <span className="muted">Latest </span>
            <strong>{chart.last?.price.toLocaleString() ?? "—"} USD</strong>
          </div>
        }
      >
        <div style={{ width: "100%", height: 260, position: "relative" }}>
          <svg viewBox="0 0 100 100" preserveAspectRatio="none" style={{ width: "100%", height: "100%" }}>
            <defs>
              <linearGradient id="areaFill" x1="0" x2="0" y1="0" y2="1">
                <stop offset="0%" stopColor="var(--accent)" stopOpacity="0.25" />
                <stop offset="100%" stopColor="var(--accent)" stopOpacity="0" />
              </linearGradient>
            </defs>
            {chart.path && (
              <>
                <path d={`${chart.path} L100,100 L0,100 Z`} fill="url(#areaFill)" stroke="none" />
                <path
                  d={chart.path}
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
              <span className="muted">Symbol</span>
              <select
                value={selectedSymbol}
                onChange={(e) => setSelectedSymbol(e.target.value)}
                style={{ width: 140 }}
              >
                {MOCK_CHART_SYMBOLS.map((s) => (
                  <option key={s} value={s}>
                    {s}
                  </option>
                ))}
              </select>
            </label>
          </div>
        </div>
        <div className="muted" style={{ marginTop: 8 }}>
          Range {chart.min.toLocaleString(undefined, { maximumFractionDigits: 2 })} –{" "}
          {chart.max.toLocaleString(undefined, { maximumFractionDigits: 2 })} USD
        </div>
      </SourcedPanel>

      <SourcedPanel
        eyebrow="Activity"
        title="Recent submissions"
        kind="local"
        endpoint="awaiting GET /orders"
        note="Acknowledgements saved by this browser. Fill status is unknown until the backend can report it."
        actions={
          <Link to="/trades">
            <button type="button" className="ghost-button">
              View all
            </button>
          </Link>
        }
      >
        <table className="table">
          <thead>
            <tr>
              <th>Order ID</th>
              <th>Market</th>
              <th>Type</th>
              <th style={{ textAlign: "right" }}>Shares</th>
              <th style={{ textAlign: "right" }}>Price</th>
              <th>Submitted</th>
            </tr>
          </thead>
          <tbody>
            {recentOrders.map((order) => {
              const legs = formatOrderLegs(reference, order);
              return (
                <tr key={`${order.order_id}-${order.submittedAt}`}>
                  <td>{order.order_id}</td>
                  <td>{order.market}</td>
                  <td>{order.type}</td>
                  <td style={{ textAlign: "right" }}>{legs.shares}</td>
                  <td style={{ textAlign: "right" }}>{legs.price}</td>
                  <td className="muted">{new Date(order.submittedAt).toLocaleString()}</td>
                </tr>
              );
            })}
            {recentOrders.length === 0 && (
              <tr>
                <td colSpan={6} className="muted">
                  No orders submitted yet. <Link to="/trades/new">Place one →</Link>
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </SourcedPanel>

      <MarketTable />
    </div>
  );
};
