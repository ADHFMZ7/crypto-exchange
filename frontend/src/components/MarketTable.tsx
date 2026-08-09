import React, { useEffect, useState } from "react";
import { SourcedPanel } from "./DataSource";
import { MOCK_MARKETS, MOCK_TICK_MS, driftMarkets } from "../mocks";
import type { MarketTicker } from "../types";

/**
 * The quote board. Every number here is invented — see mocks/index.ts.
 * Shared by the home and trades pages so the fake data has exactly one source.
 */
export const MarketTable: React.FC = () => {
  const [markets, setMarkets] = useState<MarketTicker[]>(MOCK_MARKETS);

  useEffect(() => {
    const id = setInterval(() => setMarkets(driftMarkets), MOCK_TICK_MS);
    return () => clearInterval(id);
  }, []);

  return (
    <SourcedPanel
      eyebrow="Markets"
      title="Quote board"
      kind="mock"
      endpoint="awaiting GET /markets/{symbol}/ticker"
      note={
        <>
          Invented prices on a random walk. <code>GET /markets</code> is live now, but it only says
          which pairs exist — there is still no price feed, so both the prices and these symbols come
          from <code>mocks/index.ts</code>. Only the markets listed on the trade page are tradeable.
        </>
      }
      actions={
        <button type="button" className="ghost-button" onClick={() => setMarkets(MOCK_MARKETS)}>
          Reset
        </button>
      }
    >
      <table className="table">
        <thead>
          <tr>
            <th>Symbol</th>
            <th style={{ textAlign: "right" }}>Price</th>
            <th style={{ textAlign: "right" }}>Change</th>
            <th style={{ textAlign: "right" }}>Volume</th>
          </tr>
        </thead>
        <tbody>
          {markets.map((m) => (
            <tr key={m.symbol}>
              <td>{m.symbol}</td>
              <td style={{ textAlign: "right" }}>{m.price.toLocaleString()}</td>
              <td
                style={{
                  textAlign: "right",
                  color: m.change >= 0 ? "var(--success)" : "var(--danger)"
                }}
              >
                {m.change >= 0 ? "+" : ""}
                {m.change}%
              </td>
              <td style={{ textAlign: "right" }}>{m.volume.toLocaleString()}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </SourcedPanel>
  );
};
