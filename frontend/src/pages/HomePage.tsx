import React from "react";
import { Link } from "react-router-dom";
import { useAuth } from "../hooks/useAuth";
import type { WalletBalance } from "../types";

const mockBalances: WalletBalance[] = [
  { currency: "USD", amount: 10000 },
  { currency: "BTC", amount: 0.42 },
  { currency: "ETH", amount: 3.5 }
];

export const HomePage: React.FC = () => {
  const { user } = useAuth();

  return (
    <div className="grid" style={{ gap: 18 }}>
      <div className="panel">
        <div className="headline">
          <div>
            <div className="tag">Account</div>
            <h2 style={{ margin: "4px 0" }}>{user ? `Hello, ${user.fullname}` : "Welcome"}</h2>
            <div className="muted">{user?.email}</div>
          </div>
          <Link to="/trades/new">
            <button type="button">New Trade</button>
          </Link>
        </div>
        <p className="muted">
          Your account dashboard shows a snapshot of balances and quick links to start trading. Wallet balances are mocked
          until the API supports them.
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

      <div className="grid" style={{ gridTemplateColumns: "repeat(auto-fit, minmax(240px, 1fr))" }}>
        <div className="panel">
          <div className="headline">
            <h3 style={{ margin: 0 }}>Profile</h3>
            <span className="tag">Identity</span>
          </div>
          <div className="stack">
            <div>
              <div className="muted">Name</div>
              <strong>{user?.fullname ?? "Not loaded"}</strong>
            </div>
            <div>
              <div className="muted">Email</div>
              <strong>{user?.email ?? "Not loaded"}</strong>
            </div>
          </div>
        </div>

        <div className="panel">
          <div className="headline">
            <h3 style={{ margin: 0 }}>Getting started</h3>
            <span className="tag">Steps</span>
          </div>
          <ol style={{ paddingLeft: 16, margin: 0, color: "var(--muted-text)" }}>
            <li>Sign up or log in</li>
            <li>Review wallet balances</li>
            <li>Place a new trade</li>
            <li>Track trades and market data</li>
          </ol>
        </div>
      </div>
    </div>
  );
};
