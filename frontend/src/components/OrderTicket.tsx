import React, { useEffect, useMemo, useRef, useState } from "react";
import { SourcedPanel } from "./DataSource";
import { useAuth } from "../hooks/useAuth";
import { useReference } from "../hooks/useReference";
import { api, errorMessage } from "../lib/api";
import { fromMinorUnits, parseAmountRounded, toAmount } from "../lib/decimal";
import { effectiveExponent, formatAmount } from "../lib/markets";
import {
  buildTicket,
  lockedAmount,
  lockedCurrency,
  quantityForTotal,
  quoteTotal,
  toOrderPayload
} from "../lib/order";
import type { Market } from "../lib/reference";
import type { OrderAck, Side, WalletBalance } from "../types";

const PERCENTAGES = [25, 50, 75, 100];

type Props = {
  market: Market | undefined;
  balances: WalletBalance[];
  /** Best bid and ask, in quote minor units, when the book has a touch. */
  bestBid?: number;
  bestAsk?: number;
  /** Called after a successful placement so the caller can refresh balances. */
  onPlaced: (ack: OrderAck) => void;
};

/**
 * The limit order ticket.
 *
 * Price is the first field and the widest, because on a limit order it is the
 * decision. Amount follows, and Total is solved from the two — never the other
 * way round. That ordering is the whole redesign: the previous form took two
 * amounts and derived a price, which meant the number you were actually
 * choosing never appeared as an input and had to be rounded behind your back.
 *
 * Side is an explicit toggle rather than something inferred from which currency
 * you picked. Buy and sell draw on different balances and carry different risk,
 * so it is not a choice the form should be making for you.
 */
export const OrderTicket: React.FC<Props> = ({ market, balances, bestBid, bestAsk, onPlaced }) => {
  const { token } = useAuth();
  const { reference } = useReference();

  const [side, setSide] = useState<Side>("buy");
  const [price, setPrice] = useState("");
  const [quantity, setQuantity] = useState("");

  // The price the toggle's side would cross: a buy lifts the ask, a sell hits
  // the bid. Also what seeds the empty field, so the ticket arrives useful
  // rather than as a dead 0.00 nobody can price against.
  const touch = side === "buy" ? bestAsk : bestBid;

  const seeded = useRef(false);
  useEffect(() => {
    if (seeded.current || !market || touch === undefined) return;
    seeded.current = true;
    setPrice(fromMinorUnits(BigInt(touch), effectiveExponent(reference, market.quote)));
  }, [market, reference, touch]);

  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string>();
  const [ack, setAck] = useState<OrderAck | null>(null);

  const built = useMemo(
    () => (market ? buildTicket(reference, market, side, price, quantity) : null),
    [market, quantity, price, reference, side]
  );

  const ticket = built && "ticket" in built ? built.ticket : null;
  const ticketError = built && "error" in built ? built.error : null;

  if (!market) {
    return (
      <SourcedPanel eyebrow="Order entry" title="Place a limit order" kind="live" endpoint="POST /orders">
        <div className="muted">No market selected.</div>
      </SourcedPanel>
    );
  }

  const baseExp = effectiveExponent(reference, market.base);
  const quoteExp = effectiveExponent(reference, market.quote);

  const availableOf = (code: string): bigint => {
    const balance = balances.find((b) => b.currency === code);
    return balance ? toAmount(balance.available) : 0n;
  };

  // What the order will actually cost or raise, at the typed price. Recomputed
  // from the two inputs rather than tracked as its own state, so it can never
  // drift from them.
  const total = ticket ? quoteTotal(reference, market, ticket.quantity, ticket.price) : 0n;

  const spendCurrency = lockedCurrency(market, side);
  const available = availableOf(spendCurrency);
  const locked = ticket ? lockedAmount(reference, ticket) : 0n;
  const shortfall = locked - available;
  const insufficient = shortfall > 0n;

  /** Sets the amount from a share of the balance the order will draw on. */
  const applyPercentage = (percent: number) => {
    const parsedPrice = parseAmountRounded(price, quoteExp);
    const share = (available * BigInt(percent)) / 100n;

    if (side === "sell") {
      // Selling spends base directly, so the share IS the amount.
      setQuantity(fromMinorUnits(share, baseExp));
      return;
    }
    // Buying spends quote, so the share is a budget the price converts.
    if (!parsedPrice.ok || parsedPrice.value <= 0n) return;
    setQuantity(fromMinorUnits(quantityForTotal(reference, market, share, parsedPrice.value), baseExp));
  };

  /** Editing Total solves back to an amount; Total then re-renders from it. */
  const applyTotal = (typed: string) => {
    const parsedTotal = parseAmountRounded(typed, quoteExp);
    const parsedPrice = parseAmountRounded(price, quoteExp);
    if (!parsedTotal.ok || !parsedPrice.ok || parsedPrice.value <= 0n) return;
    setQuantity(
      fromMinorUnits(quantityForTotal(reference, market, parsedTotal.value, parsedPrice.value), baseExp)
    );
  };

  /**
   * Settles both fields onto the values actually being sent. On blur rather
   * than on change, so rounding never fights someone mid-keystroke.
   */
  const settle = () => {
    if (!ticket) return;
    setPrice(fromMinorUnits(ticket.price, quoteExp));
    setQuantity(fromMinorUnits(ticket.quantity, baseExp));
  };

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!token) {
      setError("You must be logged in.");
      return;
    }
    if (!ticket) {
      setError(ticketError?.message ?? "Fill in a price and an amount.");
      return;
    }

    setSubmitting(true);
    setError(undefined);
    setAck(null);
    try {
      const placed = await api.createOrder(token, toOrderPayload(ticket));
      setAck(placed);
      setQuantity("");
      onPlaced(placed);
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setSubmitting(false);
    }
  };

  const buying = side === "buy";
  const sideColour = buying ? "var(--success)" : "var(--danger)";

  return (
    <SourcedPanel
      eyebrow="Order entry"
      title="Place a limit order"
      kind="live"
      endpoint="POST /orders"
      fill
      note={<>A <strong>202</strong> means queued for the book, not filled.</>}
    >
      <form className="ticket" onSubmit={submit}>
        <div className="side-toggle" role="group" aria-label="Order side">
          {(["buy", "sell"] as const).map((value) => (
            <button
              key={value}
              type="button"
              className={`side-option${side === value ? ` side-option-${value}` : ""}`}
              aria-pressed={side === value}
              onClick={() => setSide(value)}
            >
              {value === "buy" ? `Buy ${market.base}` : `Sell ${market.base}`}
            </button>
          ))}
        </div>

        <div className="order-type-tabs">
          <span className="order-type order-type-active">Limit</span>
          <span
            className="order-type order-type-disabled"
            title="The backend serves limit orders only — orders.quantity is required and there is no market order type yet."
          >
            Market
          </span>
          <span className="muted order-type-note">choose the price</span>
        </div>

        <label className="leg leg-price">
          <div className="leg-label">
            <span className="leg-title">Limit price</span>
            {touch === undefined ? (
              <span className="muted">
                {market.quote} per whole {market.base}
              </span>
            ) : (
              <button
                type="button"
                className="touch-button"
                onClick={() => setPrice(fromMinorUnits(BigInt(touch), quoteExp))}
                title={`Best ${buying ? "ask" : "bid"} on the book`}
              >
                Best {buying ? "ask" : "bid"} {formatAmount(reference, BigInt(touch), market.quote)}
              </button>
            )}
          </div>
          <div className="leg-row">
            <input
              className="leg-amount"
              type="text"
              inputMode="decimal"
              placeholder="0.00"
              value={price}
              onChange={(e) => setPrice(e.target.value)}
              onBlur={settle}
              aria-label={`Limit price in ${market.quote} per whole ${market.base}`}
            />
            <span className="leg-unit">{market.quote}</span>
          </div>
          <div className="muted leg-hint">
            {quoteExp === 0 ? "whole numbers only" : `up to ${quoteExp} decimals`}
          </div>
        </label>

        <label className="leg">
          <div className="leg-label">
            <span>Amount</span>
            <span className="muted">
              Available {formatAmount(reference, availableOf(market.base), market.base)}
            </span>
          </div>
          <div className="leg-row">
            <input
              className="leg-amount"
              type="text"
              inputMode="decimal"
              placeholder="0.00000000"
              value={quantity}
              onChange={(e) => setQuantity(e.target.value)}
              onBlur={settle}
              aria-label={`Amount of ${market.base}`}
            />
            <span className="leg-unit">{market.base}</span>
          </div>
          <div className="muted leg-hint">
            {baseExp === 0 ? "whole numbers only" : `up to ${baseExp} decimals`}
          </div>
        </label>

        <div className="percentages">
          {PERCENTAGES.map((percent) => (
            <button
              key={percent}
              type="button"
              className="percentage"
              onClick={() => applyPercentage(percent)}
              title={`${percent}% of your available ${spendCurrency}`}
            >
              {percent}%
            </button>
          ))}
        </div>

        <label className="leg">
          <div className="leg-label">
            <span>Total</span>
            <span className="muted">
              Available {formatAmount(reference, availableOf(market.quote), market.quote)}
            </span>
          </div>
          <div className="leg-row">
            <input
              className="leg-amount"
              type="text"
              inputMode="decimal"
              placeholder="0.00"
              value={total > 0n ? fromMinorUnits(total, quoteExp) : ""}
              onChange={(e) => applyTotal(e.target.value)}
              aria-label={`Total in ${market.quote}`}
            />
            <span className="leg-unit">{market.quote}</span>
          </div>
          <div className="muted leg-hint">
            {buying ? "What this locks, rounded up as the ledger does" : "What this raises at your limit"}
          </div>
        </label>

        {ticket && (
          <div className="rate-line">
            <span>
              {buying ? "Locks" : "Locks"}{" "}
              <strong>{formatAmount(reference, locked, spendCurrency)}</strong> until it fills or is
              cancelled
            </span>
            <span className="muted">
              {buying ? "Rests below the best ask" : "Rests above the best bid"}
            </span>
          </div>
        )}

        {ticketError && quantity !== "" && price !== "" && (
          <div className="pill status-danger">{ticketError.message}</div>
        )}

        {insufficient && (
          <div className="pill status-danger">
            Short {formatAmount(reference, shortfall, spendCurrency)} — you have{" "}
            {formatAmount(reference, available, spendCurrency)}.
          </div>
        )}

        {error && <div className="pill status-danger">{error}</div>}

        <button
          type="submit"
          className="submit-order"
          style={{ background: sideColour, borderColor: sideColour }}
          disabled={submitting || !ticket || insufficient}
        >
          {submitting
            ? "Placing…"
            : ticket
              ? `${buying ? "Buy" : "Sell"} ${fromMinorUnits(ticket.quantity, baseExp)} ${market.base}`
              : `${buying ? "Buy" : "Sell"} ${market.base}`}
        </button>

        {ack && (
          <div className="rate-line" style={{ background: "color-mix(in srgb, var(--success) 8%, var(--muted))", borderColor: "color-mix(in srgb, var(--success) 30%, var(--border))" }}>
            <span>
              Order <strong>#{ack.order_id}</strong> accepted — <strong>{ack.status}</strong>
            </span>
            <span className="muted">queued for the book, not filled</span>
          </div>
        )}
      </form>
    </SourcedPanel>
  );
};
