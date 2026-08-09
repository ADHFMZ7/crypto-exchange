import React, { useCallback, useEffect, useState } from "react";
import { SourcedPanel } from "../components/DataSource";
import { useAuth } from "../hooks/useAuth";
import { useReference } from "../hooks/useReference";
import { ApiError, api, errorMessage } from "../lib/api";
import { parseAmountRounded, toAmount } from "../lib/decimal";
import { effectiveExponent, formatAmount, formatBalance } from "../lib/markets";
import type { WalletBalance } from "../types";

export const WalletPage: React.FC = () => {
  const { token, logout } = useAuth();
  const { reference } = useReference();
  const [balances, setBalances] = useState<WalletBalance[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string>();

  const [amount, setAmount] = useState("100");
  const [pending, setPending] = useState(false);
  const [transferError, setTransferError] = useState<string>();
  const [transferNote, setTransferNote] = useState<string>();

  const loadWallet = useCallback(async () => {
    if (!token) return;
    setLoading(true);
    setError(undefined);
    try {
      const wallet = await api.getWallet(token);
      setBalances(wallet.balances ?? []);
    } catch (err) {
      if (err instanceof ApiError && err.isUnauthorized) {
        logout();
        return;
      }
      setError(errorMessage(err));
    } finally {
      setLoading(false);
    }
  }, [logout, token]);

  useEffect(() => {
    loadWallet();
  }, [loadWallet]);

  // Dollars are a display convention that exists in this input and nowhere
  // else. It is parsed straight from the typed string — Number("0.1") * 100 is
  // 10.000000000000002, and a balance built out of that will not reconcile.
  const usdExponent = effectiveExponent(reference, "USD");
  const parsedTransfer = parseAmountRounded(amount, usdExponent);
  const transferMinor = parsedTransfer.ok ? parsedTransfer.value : 0n;

  const submitTransfer = async (direction: 1 | -1) => {
    if (!token) return;

    if (!parsedTransfer.ok || transferMinor <= 0n) {
      setTransferError("Enter a positive amount.");
      return;
    }

    setPending(true);
    setTransferError(undefined);
    setTransferNote(undefined);
    try {
      await api.deposit(token, Number(transferMinor) * direction);
      setTransferNote(
        `${direction === 1 ? "Deposited" : "Withdrew"} ${formatAmount(reference, transferMinor, "USD")}.`
      );
      await loadWallet();
    } catch (err) {
      setTransferError(errorMessage(err));
    } finally {
      setPending(false);
    }
  };

  const totalLocked = balances.reduce((sum, b) => sum + toAmount(b.locked), 0n);

  return (
    <div className="grid" style={{ gap: 18 }}>
      <SourcedPanel
        eyebrow="Wallet"
        title="Balances"
        kind="live"
        endpoint="GET /wallets/me"
        note="Every figure below is read from the database on page load."
        actions={
          <button type="button" className="ghost-button" onClick={loadWallet} disabled={loading}>
            {loading ? "Refreshing…" : "Refresh"}
          </button>
        }
      >
        {error && <div className="pill status-danger">{error}</div>}

        {!error && (
          <table className="table" style={{ marginTop: 12 }}>
            <thead>
              <tr>
                <th>Currency</th>
                <th style={{ textAlign: "right" }}>Available</th>
                <th style={{ textAlign: "right" }}>Locked in orders</th>
              </tr>
            </thead>
            <tbody>
              {balances.map((balance) => (
                <tr key={balance.id}>
                  <td>
                    <strong>{balance.currency}</strong>
                  </td>
                  <td style={{ textAlign: "right" }}>
                    {formatBalance(reference, balance.available, balance.currency)}
                  </td>
                  <td style={{ textAlign: "right" }} className={balance.locked ? undefined : "muted"}>
                    {formatBalance(reference, balance.locked, balance.currency)}
                  </td>
                </tr>
              ))}
              {balances.length === 0 && !loading && (
                <tr>
                  <td colSpan={3} className="muted">
                    No balances yet.
                  </td>
                </tr>
              )}
              {loading && balances.length === 0 && (
                <tr>
                  <td colSpan={3} className="muted">
                    Loading balances…
                  </td>
                </tr>
              )}
            </tbody>
          </table>
        )}

        {totalLocked === 0n && balances.length > 0 && (
          <div className="muted" style={{ marginTop: 10 }}>
            Locked reads 0 for every currency. <code>GetByUserID</code> selects only{" "}
            <code>available</code>, so this column cannot show a non-zero value until the query
            includes <code>locked</code>.
          </div>
        )}
      </SourcedPanel>

      <SourcedPanel
        eyebrow="Transfer"
        title="Deposit or withdraw USD"
        kind="live"
        endpoint="PATCH /wallets/me"
        note={
          <>
            Sends a signed delta in <strong>cents</strong> — withdrawals are the same call with a
            negative amount. Dollars exist in the field below and nowhere past it.
          </>
        }
      >
        <div className="inline-actions" style={{ gap: 12, alignItems: "flex-end" }}>
          <label className="stack" style={{ flex: 1, gap: 6 }}>
            <span>Amount (USD)</span>
            <input
              type="text"
              inputMode="decimal"
              value={amount}
              onChange={(e) => setAmount(e.target.value)}
              aria-label="Amount in USD to deposit or withdraw"
            />
          </label>
          <button type="button" onClick={() => submitTransfer(1)} disabled={pending}>
            {pending ? "Working…" : "Deposit"}
          </button>
          <button
            type="button"
            className="ghost-button"
            onClick={() => submitTransfer(-1)}
            disabled={pending}
          >
            Withdraw
          </button>
        </div>

        <div className="muted" style={{ marginTop: 10 }}>
          {parsedTransfer.ok && transferMinor > 0n ? (
            <>
              Sent as <code>{transferMinor.toString()}</code> cents
              {parsedTransfer.rounded && " — USD holds 2 decimals, so that was rounded"}.
            </>
          ) : (
            <>Enter a positive amount — USD holds 2 decimals, so a cent is the smallest unit.</>
          )}
        </div>

        {transferError && (
          <div className="pill status-danger" style={{ marginTop: 12 }}>
            {transferError}
          </div>
        )}
        {transferNote && (
          <div className="pill status-success" style={{ marginTop: 12 }}>
            {transferNote}
          </div>
        )}
      </SourcedPanel>
    </div>
  );
};
