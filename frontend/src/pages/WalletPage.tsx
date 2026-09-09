import React, { useCallback, useState } from "react";
import { SourcedPanel } from "../components/DataSource";
import { useAuth } from "../hooks/useAuth";
import { usePolling } from "../hooks/usePolling";
import { useReference } from "../hooks/useReference";
import { ApiError, api, errorMessage } from "../lib/api";
import { parseAmountRounded, toAmount } from "../lib/decimal";
import { effectiveExponent, formatAmount, formatBalance, minorUnitName } from "../lib/markets";
import type { WalletBalance } from "../types";

export const WalletPage: React.FC = () => {
  const { token, logout } = useAuth();
  const { reference } = useReference();
  const [balances, setBalances] = useState<WalletBalance[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string>();

  const [transferCurrency, setTransferCurrency] = useState("USD");
  const [amount, setAmount] = useState("100");
  const [pending, setPending] = useState(false);
  const [transferError, setTransferError] = useState<string>();
  const [transferNote, setTransferNote] = useState<string>();

  // Balances move without the user doing anything now: settlement shifts funds
  // between locked and available as orders fill. `silent` keeps a background
  // poll from flickering the button.
  const loadWallet = useCallback(
    async (silent = false) => {
      if (!token) return;
      if (!silent) setLoading(true);
      try {
        const wallet = await api.getWallet(token);
        setBalances(wallet.balances ?? []);
        setError(undefined);
      } catch (err) {
        if (err instanceof ApiError && err.isUnauthorized) {
          logout();
          return;
        }
        setError(errorMessage(err));
      } finally {
        if (!silent) setLoading(false);
      }
    },
    [logout, token]
  );

  usePolling(() => loadWallet(true), 6000, Boolean(token));

  // Whole units are a display convention that exists in this input and nowhere
  // else. The typed string is parsed directly — Number("0.1") * 100 is
  // 10.000000000000002, and a balance built out of that will not reconcile.
  //
  // Which minor unit it parses into depends on the selected currency: "0.5"
  // is 50 cents in USD and 50,000,000 satoshis in BTC. Reading the exponent
  // from the currency rather than assuming one is the whole point.
  const transferExponent = effectiveExponent(reference, transferCurrency);
  const parsedTransfer = parseAmountRounded(amount, transferExponent);
  const transferMinor = parsedTransfer.ok ? parsedTransfer.value : 0n;

  // Every currency the exchange lists, not just the ones already held — the
  // point is to fund a balance that does not exist yet.
  const currencyOptions = Object.keys(reference.currencies).sort();
  const minorUnitLabel = minorUnitName(transferCurrency, transferExponent);

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
      await api.deposit(token, transferCurrency, Number(transferMinor) * direction);
      setTransferNote(
        `${direction === 1 ? "Deposited" : "Withdrew"} ${formatAmount(reference, transferMinor, transferCurrency)}.`
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
        note="Read from the database, and refreshed on a timer — settlement moves funds between locked and available as orders fill."
        actions={
          <button
            type="button"
            className="ghost-button"
            onClick={() => loadWallet()}
            disabled={loading}
          >
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

        {totalLocked > 0n && (
          <div className="muted" style={{ marginTop: 10 }}>
            Locked funds are committed to open orders. Settlement releases them as fills execute —
            the spent portion moves to the counterparty, and anything locked above what the trade
            actually cost returns to available when the order completes.
          </div>
        )}
      </SourcedPanel>

      <SourcedPanel
        eyebrow="Transfer"
        title="Deposit or withdraw"
        kind="live"
        endpoint="PATCH /wallets/me"
        note={
          <>
            Sends a signed delta in the selected currency's <strong>minor units</strong> —
            withdrawals are the same call with a negative amount. Depositing a currency you have
            never held creates the balance, which is how you fund the sell side of a market.
          </>
        }
      >
        <div className="inline-actions" style={{ gap: 12, alignItems: "flex-end" }}>
          <label className="stack" style={{ gap: 6 }}>
            <span>Currency</span>
            <select
              value={transferCurrency}
              onChange={(e) => setTransferCurrency(e.target.value)}
              aria-label="Currency to deposit or withdraw"
            >
              {currencyOptions.map((code) => (
                <option key={code} value={code}>
                  {code} — {reference.currencies[code]?.name ?? code}
                </option>
              ))}
            </select>
          </label>
          <label className="stack" style={{ flex: 1, gap: 6 }}>
            <span>Amount ({transferCurrency})</span>
            <input
              type="text"
              inputMode="decimal"
              value={amount}
              onChange={(e) => setAmount(e.target.value)}
              aria-label={`Amount in ${transferCurrency} to deposit or withdraw`}
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
              Sent as <code>{transferMinor.toString()}</code> {minorUnitLabel}
              {parsedTransfer.rounded &&
                ` — ${transferCurrency} holds ${transferExponent} decimals, so that was rounded`}
              .
            </>
          ) : (
            <>
              Enter a positive amount — {transferCurrency} holds {transferExponent} decimals, so one{" "}
              {minorUnitLabel.replace(/s$/, "")} is the smallest unit.
            </>
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
