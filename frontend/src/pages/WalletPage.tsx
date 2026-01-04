import React from "react";
import type { WalletBalance } from "../types";

// Placeholder balances until a real wallet endpoint is wired up.
const mockBalances: WalletBalance[] = [
  { currency: "USD", amount: 10000 },
  { currency: "BTC", amount: 0.42 },
  { currency: "ETH", amount: 3.5 },
  { currency: "SOL", amount: 50 }
];

export const WalletPage: React.FC = () => {
  return (
    <div className="grid" style={{ gap: 18 }}>
      <div className="panel">
        <div className="headline">
          <div>
            <div className="tag">Wallet</div>
            <h2 style={{ margin: "4px 0" }}>Balances</h2>
            <div className="muted">Showing mocked balances until the wallet API is ready.</div>
          </div>
          <div className="inline-actions">
            <button type="button" className="ghost-button">
              Deposit (stub)
            </button>
            <button type="button" className="ghost-button">
              Withdraw (stub)
            </button>
          </div>
        </div>

        <table className="table" style={{ marginTop: 12 }}>
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
          <h3 style={{ margin: 0 }}>Next steps</h3>
          <span className="tag">Integration</span>
        </div>
        <ul style={{ margin: 0, paddingLeft: 16, color: "var(--muted-text)" }}>
          <li>Replace mocks with `GET /wallets/me` and render live balances.</li>
          <li>Wire deposit/withdraw buttons to backend endpoints when available.</li>
          <li>Show balance history or recent transfers for transparency.</li>
        </ul>
      </div>
    </div>
  );
};
