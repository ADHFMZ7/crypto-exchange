import React, { useEffect, useState } from "react";
import { useAuth } from "../hooks/useAuth";
import type { Wallet, WalletBalance } from "../types";

const API_BASE = import.meta.env.VITE_API_BASE_URL ?? "http://localhost:8080";

export const WalletPage: React.FC = () => {
  const { token } = useAuth();
  const [balances, setBalances] = useState<WalletBalance[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string>();

  useEffect(() => {
    const fetchBalances = async () => {
      if (!token) return;
      setLoading(true);
      setError(undefined);
      try {
        const res = await fetch(`${API_BASE}/wallets/me`, {
          headers: {
            Authorization: `Bearer ${token}`
          }
        });
        if (!res.ok) {
          throw new Error(`Failed to fetch wallet: ${res.status}`);
        }
        const data: Wallet = await res.json();
        setBalances(data.balances ?? []);
      } catch (err) {
        console.error(err);
        setError(err instanceof Error ? err.message : "Failed to load wallet");
      } finally {
        setLoading(false);
      }
    };

    fetchBalances();
  }, [token]);

  return (
    <div className="grid" style={{ gap: 18 }}>
      <div className="panel">
        <div className="headline">
          <div>
            <div className="tag">Wallet</div>
            <h2 style={{ margin: "4px 0" }}>Balances</h2>
            <div className="muted">Live balances from the wallet API.</div>
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

        {loading && <div className="pill">Loading balances...</div>}
        {error && <div className="pill status-danger">{error}</div>}
        {!loading && !error && (
          <table className="table" style={{ marginTop: 12 }}>
            <thead>
              <tr>
                <th>Currency</th>
                <th>Balance</th>
              </tr>
            </thead>
            <tbody>
              {balances.map((balance) => (
                <tr key={balance.currency}>
                  <td>{balance.currency}</td>
                  <td>{balance.amount}</td>
                </tr>
              ))}
              {balances.length === 0 && (
                <tr>
                  <td colSpan={2} className="muted">
                    No balances yet.
                  </td>
                </tr>
              )}
            </tbody>
          </table>
        )}
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
