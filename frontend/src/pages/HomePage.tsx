import React, { useCallback, useEffect, useMemo, useState } from "react";
import { Link } from "react-router-dom";
import { SourcedPanel } from "../components/DataSource";
import { StreamBadge } from "../components/StreamBadge";
import { MarketTable } from "../components/MarketTable";
import { useAuth } from "../hooks/useAuth";
import { useReference } from "../hooks/useReference";
import { ApiError, api, errorMessage } from "../lib/api";
import { toAmount } from "../lib/decimal";
import { fillFraction, formatBalance, formatOrderLegs } from "../lib/markets";
import { useMarketStream } from "../hooks/useMarketStream";
import { usePolling } from "../hooks/usePolling";
import { formatPrice } from "../lib/markets";
import type { MarketTrade, Order, WalletBalance } from "../types";

export const HomePage: React.FC = () => {
  const { user, token, logout } = useAuth();
  const { reference } = useReference();

  const [balances, setBalances] = useState<WalletBalance[]>([]);
  const [walletError, setWalletError] = useState<string>();
  const [walletLoading, setWalletLoading] = useState(false);

  const [orders, setOrders] = useState<Order[]>([]);
  const [selectedSymbol, setSelectedSymbol] = useState<string>(
    reference.markets[0]?.symbol ?? ""
  );
  const [tape, setTape] = useState<MarketTrade[]>([]);


  useEffect(() => {
    if (!token) return;
    let cancelled = false;

    (async () => {
      setWalletLoading(true);
      setWalletError(undefined);
      try {
        // Both are the caller's own state and share a failure mode, so one
        // round trip pair keeps the summary internally consistent.
        const [wallet, orderList] = await Promise.all([
          api.getWallet(token),
          api.getOrders(token)
        ]);
        if (cancelled) return;
        setBalances(wallet.balances ?? []);
        setOrders(orderList.orders ?? []);
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

  /*
   * The chart is the trade tape plotted against time, not candles.
   *
   * There is no candle endpoint, and inventing one client-side would mean
   * choosing a bucket size and then presenting the result as if the server had
   * said it. Executions are what actually happened, so they are what is drawn —
   * irregularly spaced, because that is what a real book does when nobody
   * trades for a minute.
   */
  const loadTape = useCallback(async () => {
    if (!selectedSymbol) return;
    try {
      const res = await api.getMarketTrades(selectedSymbol, 60);
      setTape(res.trades ?? []);
    } catch {
      // The panel renders its own empty state; a failed poll is not worth a
      // banner on the landing page.
    }
  }, [selectedSymbol]);

  // The chart is the same executions the tape shows, so it takes the same feed.
  // The snapshot is REST; everything after arrives pushed.
  const streamStatus = useMarketStream(
    selectedSymbol,
    (trade) =>
      setTape((prev) => (prev.some((t) => t.id === trade.id) ? prev : [trade, ...prev].slice(0, 60))),
    loadTape
  );

  usePolling(loadTape, 8000, Boolean(selectedSymbol) && streamStatus !== "live");

  // Oldest first: the tape arrives newest first, and a chart reads left to right.
  const currentSeries = useMemo(
    () => tape.map((t) => ({ t: Date.parse(t.executed_at), price: t.price })).reverse(),
    [tape]
  );
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
        devNote="Identity and balances read from Postgres."
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
        title={selectedSymbol || "Markets"}
        kind="live"
        endpoint="GET /markets/{symbol}/trades"
        note={<>Every trade on this market, oldest on the left.</>}
        devNote={
          <>
            Raw executions from <code>GET /markets/{"{symbol}"}/trades</code>, not candles — there
            is no endpoint for those, and bucketing them here would mean inventing an interval and
            presenting it as if the server had chosen it.
          </>
        }
        actions={
          <>
            <StreamBadge status={streamStatus} />
            <div className="pill">
              <span className="muted">Last </span>
              <strong>
                {chart.last ? formatPrice(reference, selectedSymbol, chart.last.price) : "—"}
              </strong>
            </div>
          </>
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
                {reference.markets.map((m) => (
                  <option key={m.symbol} value={m.symbol}>
                    {m.symbol}
                  </option>
                ))}
              </select>
            </label>
          </div>
        </div>
        <div className="muted" style={{ marginTop: 8 }}>
          {currentSeries.length === 0 ? (
            <>Nothing has traded on {selectedSymbol || "this market"} yet — the chart fills in as orders cross.</>
          ) : (
            <>
              {currentSeries.length} execution{currentSeries.length === 1 ? "" : "s"}, ranging{" "}
              {formatPrice(reference, selectedSymbol, chart.min)} –{" "}
              {formatPrice(reference, selectedSymbol, chart.max)}
            </>
          )}
        </div>
      </SourcedPanel>

      <SourcedPanel
        eyebrow="Activity"
        title="Recent orders"
        kind="live"
        endpoint="GET /orders"
        note="Your five most recent orders."
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
            {recentOrders.map((order) => {
              const legs = formatOrderLegs(reference, order);
              const filled = fillFraction(order);
              return (
                <tr key={order.id}>
                  <td>{order.id}</td>
                  <td>{order.market}</td>
                  <td style={{ color: order.side === "buy" ? "var(--success)" : "var(--danger)" }}>
                    {order.side}
                  </td>
                  <td style={{ textAlign: "right" }}>{legs.quantity}</td>
                  <td style={{ textAlign: "right" }} className={filled ? undefined : "muted"}>
                    {Math.round(filled * 100)}%
                  </td>
                  <td style={{ textAlign: "right" }}>{legs.price}</td>
                  <td>
                    <span className="tag">{order.status}</span>
                  </td>
                  <td className="muted">{new Date(order.created_at).toLocaleString()}</td>
                </tr>
              );
            })}
            {recentOrders.length === 0 && (
              <tr>
                <td colSpan={8} className="muted">
                  No orders yet. <Link to="/trades/new">Place one →</Link>
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
