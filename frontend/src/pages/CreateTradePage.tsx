import React, { useMemo, useState } from "react";
import { useAuth } from "../hooks/useAuth";

type TradeType = "limit_buy" | "limit_sell" | "cancel";
type SubmitResult = {
  status: string;
  order_id: number;
  market: string;
  type: string;
  receivedAt: string;
};

const API_BASE = import.meta.env.VITE_API_BASE_URL ?? "http://localhost:8080";
const supportedMarkets = ["BTC-USD"];

export const CreateTradePage: React.FC = () => {
  const { token } = useAuth();
  const [market, setMarket] = useState(supportedMarkets[0]);
  const [type, setType] = useState<TradeType>("limit_buy");
  const [shares, setShares] = useState(1);
  const [price, setPrice] = useState(50000);
  const [cancelOrderID, setCancelOrderID] = useState("");
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string>();
  const [result, setResult] = useState<SubmitResult | null>(null);

  const isCancel = type === "cancel";
  const notional = useMemo(() => shares * price, [price, shares]);

  const onSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!token) {
      setError("You must be logged in.");
      return;
    }

    const payload: Record<string, unknown> = {
      market,
      type
    };

    if (isCancel) {
      const parsedID = Number(cancelOrderID);
      if (!Number.isInteger(parsedID) || parsedID <= 0) {
        setError("Cancel requests require a valid positive order ID.");
        return;
      }
      payload.order_id = parsedID;
    } else {
      if (!Number.isInteger(shares) || !Number.isInteger(price) || shares <= 0 || price <= 0) {
        setError("Shares and price must be positive integers.");
        return;
      }
      payload.shares = shares;
      payload.price = price;
    }

    setLoading(true);
    setError(undefined);
    setResult(null);

    try {
      const res = await fetch(`${API_BASE}/trades`, {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
          Authorization: `Bearer ${token}`
        },
        body: JSON.stringify(payload)
      });

      if (!res.ok) {
        const message = await res.text();
        throw new Error(message || `Request failed: ${res.status}`);
      }

      const data = (await res.json()) as SubmitResult;
      setResult(data);
      if (isCancel) {
        setCancelOrderID("");
      }
    } catch (err) {
      console.error(err);
      setError(err instanceof Error ? err.message : "Failed to submit trade request");
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="grid" style={{ gap: 18 }}>
      <div className="panel">
        <div className="headline" style={{ alignItems: "flex-start" }}>
          <div className="stack" style={{ gap: 6 }}>
            <div className="tag">Trade API</div>
            <h2 style={{ margin: 0 }}>Submit trade request</h2>
            <div className="muted">Connected to `POST /trades`. Current backend queue supports `BTC-USD`.</div>
          </div>
        </div>

        <form className="stack" style={{ gap: 14 }} onSubmit={onSubmit}>
          <label className="stack">
            <span>Market</span>
            <select value={market} onChange={(e) => setMarket(e.target.value)}>
              {supportedMarkets.map((symbol) => (
                <option key={symbol} value={symbol}>
                  {symbol}
                </option>
              ))}
            </select>
          </label>

          <label className="stack">
            <span>Type</span>
            <select value={type} onChange={(e) => setType(e.target.value as TradeType)}>
              <option value="limit_buy">Limit Buy</option>
              <option value="limit_sell">Limit Sell</option>
              <option value="cancel">Cancel</option>
            </select>
          </label>

          {isCancel ? (
            <label className="stack">
              <span>Order ID to cancel</span>
              <input
                type="number"
                min={1}
                step={1}
                value={cancelOrderID}
                onChange={(e) => setCancelOrderID(e.target.value)}
                placeholder="e.g. 42"
                required
              />
            </label>
          ) : (
            <div className="inline-actions">
              <label className="stack" style={{ flex: 1 }}>
                <span>Shares (int64)</span>
                <input
                  type="number"
                  min={1}
                  step={1}
                  value={shares}
                  onChange={(e) => setShares(Number(e.target.value))}
                  required
                />
              </label>

              <label className="stack" style={{ flex: 1 }}>
                <span>Price (int64)</span>
                <input
                  type="number"
                  min={1}
                  step={1}
                  value={price}
                  onChange={(e) => setPrice(Number(e.target.value))}
                  required
                />
              </label>
            </div>
          )}

          {!isCancel && (
            <div className="pill">
              <strong>Notional:</strong> {notional.toLocaleString()}
            </div>
          )}

          {error && <div className="pill status-danger">{error}</div>}
          <button type="submit" disabled={loading}>
            {loading ? "Submitting..." : "Submit request"}
          </button>
        </form>
      </div>

      {result && (
        <div className="panel">
          <div className="headline">
            <h3 style={{ margin: 0 }}>Server response</h3>
            <span className="tag status-success">{result.status}</span>
          </div>
          <div className="stack">
            <div className="card">
              <div className="muted">Order ID</div>
              <strong>{result.order_id}</strong>
            </div>
            <div className="card">
              <div className="muted">Market</div>
              <strong>{result.market}</strong>
            </div>
            <div className="card">
              <div className="muted">Type</div>
              <strong>{result.type}</strong>
            </div>
            <div className="muted">Received at: {new Date(result.receivedAt).toLocaleString()}</div>
          </div>
        </div>
      )}
    </div>
  );
};
