import React, { useMemo, useState } from "react";
import type { Trade } from "../types";

const currencies = ["USD", "BTC", "ETH", "SOL", "DOGE"];

export const CreateTradePage: React.FC = () => {
  const [giveCurrency, setGiveCurrency] = useState("USD");
  const [receiveCurrency, setReceiveCurrency] = useState("BTC");
  const [side, setSide] = useState<"buy" | "sell">("buy");
  const [quantity, setQuantity] = useState(0.25);
  const [price, setPrice] = useState(50000);
  const [preview, setPreview] = useState<Trade | null>(null);

  const market = useMemo(() => `${receiveCurrency}-${giveCurrency}`, [giveCurrency, receiveCurrency]);
  const primaryLabel = side === "buy" ? "Pay with" : "Sell from";
  const secondaryLabel = side === "buy" ? "Buy" : "Receive in";
  const quantityLabel = side === "buy" ? "Amount to spend" : "Amount to sell";
  const priceLabel =
    side === "buy"
      ? `Limit price (per ${receiveCurrency} in ${giveCurrency})`
      : `Limit price (per ${giveCurrency} in ${receiveCurrency})`;
  const notionalLabel = side === "buy" ? "Estimated cost" : "Estimated proceeds";
  const notionalCurrency = side === "buy" ? giveCurrency : receiveCurrency;
  const priceDenomination = side === "buy" ? giveCurrency : receiveCurrency;

  const onSwap = () => {
    setGiveCurrency(receiveCurrency);
    setReceiveCurrency(giveCurrency);
    setSide((prev) => (prev === "buy" ? "sell" : "buy"));
  };

  const onSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    const trade: Trade = {
      id: crypto.randomUUID(),
      market,
      side,
      quantity,
      price,
      status: "open",
      placedAt: new Date().toISOString()
    };
    setPreview(trade);
  };

  return (
    <div className="grid" style={{ gap: 18 }}>
      <div className="panel">
        <div className="headline" style={{ alignItems: "flex-start" }}>
          <div className="stack" style={{ gap: 6 }}>
            <div className="tag">Trade ticket</div>
            <h2 style={{ margin: 0 }}>Create a trade</h2>
            <div className="muted">This is a front-end only template until trading APIs exist.</div>
          </div>
          <div className="inline-actions" style={{ gap: 8 }}>
            <button
              type="button"
              onClick={() => setSide("buy")}
              style={{
                background: side === "buy" ? "var(--accent)" : "rgba(255,255,255,0.05)",
                color: side === "buy" ? "#0b1020" : "var(--text)",
                padding: "8px 12px"
              }}
            >
              Buy
            </button>
            <button
              type="button"
              onClick={() => setSide("sell")}
              style={{
                background: side === "sell" ? "var(--accent)" : "rgba(255,255,255,0.05)",
                color: side === "sell" ? "#0b1020" : "var(--text)",
                padding: "8px 12px"
              }}
            >
              Sell
            </button>
          </div>
        </div>

        <form className="stack" style={{ gap: 14 }} onSubmit={onSubmit}>
          <div className="inline-actions" style={{ alignItems: "flex-end" }}>
            <label className="stack" style={{ flex: 1 }}>
              <span>{primaryLabel}</span>
              <select value={giveCurrency} onChange={(e) => setGiveCurrency(e.target.value)}>
                {currencies.map((c) => (
                  <option key={c} value={c}>
                    {c}
                  </option>
                ))}
              </select>
            </label>

            <button type="button" className="ghost-button" onClick={onSwap} style={{ alignSelf: "stretch" }}>
              Swap
            </button>

            <label className="stack" style={{ flex: 1 }}>
              <span>{secondaryLabel}</span>
              <select value={receiveCurrency} onChange={(e) => setReceiveCurrency(e.target.value)}>
                {currencies.map((c) => (
                  <option key={c} value={c}>
                    {c}
                  </option>
                ))}
              </select>
            </label>
          </div>

          <div className="pill" style={{ textAlign: "center" }}>
            Market: {market}
          </div>

          <div className="inline-actions">
            <label className="stack" style={{ flex: 1 }}>
              <span>{quantityLabel}</span>
              <input
                type="number"
                min={0}
                step={0.01}
                value={quantity}
                onChange={(e) => setQuantity(Number(e.target.value))}
              />
            </label>

            <label className="stack" style={{ flex: 1 }}>
              <span>{priceLabel}</span>
              <input
                type="number"
                min={0}
                step={0.01}
                value={price}
                onChange={(e) => setPrice(Number(e.target.value))}
              />
            </label>
          </div>

          <div className="pill">
            <strong>{notionalLabel}: </strong>{" "}
            {(quantity * price).toLocaleString(undefined, { maximumFractionDigits: 2 })} {notionalCurrency}
          </div>

          <button type="submit">Preview {side === "buy" ? "buy" : "sell"} order</button>
        </form>
      </div>

      {preview && (
        <div className="panel">
          <div className="headline">
            <h3 style={{ margin: 0 }}>Preview</h3>
            <div className="tag">{preview.status}</div>
          </div>
          <div className="stack">
            <div className="card">
              <div className="muted">Market</div>
              <strong>{preview.market}</strong>
            </div>
            <div className="card">
              <div className="muted">Side</div>
              <strong style={{ color: preview.side === "buy" ? "var(--success)" : "var(--danger)" }}>{preview.side}</strong>
            </div>
            <div className="card">
              <div className="muted">Give / Receive</div>
              <strong>
                {quantity} {giveCurrency} ➜ {receiveCurrency} @ {price} {priceDenomination}
              </strong>
            </div>
            <div className="muted">
              Submission is local only. Wire this to the backend trade endpoint once available, and pipe the response into
              the trades list.
            </div>
          </div>
        </div>
      )}
    </div>
  );
};
