import type { Market, ReferenceData } from "./reference";
import type { Side } from "../types";
import { divCeil, divFloor, parseAmountRounded, pow10 } from "./decimal";
import { effectiveExponent } from "./markets";

/**
 * The limit ticket's arithmetic.
 *
 * A limit order is two integers and a side, and this module is the only place
 * that converts between them and the third number a person actually thinks in —
 * the total. It replaced `buildIntent`, which went the other way: it took two
 * amounts and derived a price. Deriving the price is what hid it from the user
 * and what forced the "nudged to X" apology, because a derived price has to be
 * rounded and then one of the amounts has to move to match it.
 *
 * Here price is an input. Nothing is derived behind anyone's back, and the only
 * rounding left is the rounding Postgres would do anyway.
 *
 * EVERY VALUE HERE IS INTEGER MINOR UNITS. `quantity` is base minor units
 * (satoshis on BTC-USD); `price` is quote minor units per one WHOLE base unit
 * (cents per whole BTC); `total` is quote minor units (cents).
 */

/** A limit order, resolved and ready for the wire. */
export type LimitTicket = {
  market: Market;
  side: Side;
  quantity: bigint;
  price: bigint;
};

export type TicketError = {
  /** Which input to point at, so the form can mark the right field. */
  field: "price" | "quantity" | "balance";
  message: string;
};

/**
 * The quote amount a fill of this size at this price moves.
 *
 * Rounded UP, because that is what the backend does — `market.notional(...,
 * roundUp)` — for both sides of a trade. Settlement computes this figure once
 * and uses the same number twice: the buyer is debited it and the seller is
 * credited it, which is how the four balance moves sum to zero. Rounding down
 * here would show a buyer a total a cent below what actually gets locked.
 */
export function quoteTotal(
  ref: ReferenceData,
  market: Market,
  quantity: bigint,
  price: bigint
): bigint {
  if (quantity <= 0n || price <= 0n) return 0n;
  return divCeil(quantity * price, pow10(effectiveExponent(ref, market.base)));
}

/**
 * The largest quantity whose total does not exceed `total`.
 *
 * Rounded DOWN on purpose: this backs the Total field and the percentage
 * buttons, and both mean "spend at most this". Rounding up would let a 100%
 * button build an order the balance cannot cover.
 */
export function quantityForTotal(
  ref: ReferenceData,
  market: Market,
  total: bigint,
  price: bigint
): bigint {
  if (total <= 0n || price <= 0n) return 0n;
  return divFloor(total * pow10(effectiveExponent(ref, market.base)), price);
}

/** Which currency a side's funds are held in while the order rests. */
export function lockedCurrency(market: Market, side: Side): string {
  return side === "buy" ? market.quote : market.base;
}

/**
 * What placing this order takes out of the available balance.
 *
 * A buy locks the quote it may spend; a sell locks the base it is selling, one
 * for one with the quantity. Mirrors `market.Spends` on the backend, which is
 * what will actually be checked — so a form that agrees with this never sends
 * an order that comes back "Insufficient funds".
 */
export function lockedAmount(ref: ReferenceData, ticket: LimitTicket): bigint {
  return ticket.side === "buy"
    ? quoteTotal(ref, ticket.market, ticket.quantity, ticket.price)
    : ticket.quantity;
}

/**
 * Builds the POST /orders body. The single place the wire format is constructed.
 *
 * TODO(docs/api-todos.md § 1c): amounts become strings when minor-unit values
 * outgrow the IEEE-754 safe range — satoshis fit today, an 18-decimal currency
 * would not. Nothing outside this function changes when they do.
 */
export function toOrderPayload(ticket: LimitTicket): {
  market: string;
  side: Side;
  quantity: number;
  price: number;
} {
  return {
    market: ticket.market.symbol,
    side: ticket.side,
    quantity: Number(ticket.quantity),
    price: Number(ticket.price)
  };
}

/**
 * Turns what is typed into a ticket, or says which field is wrong.
 *
 * Excess precision is rounded rather than rejected — the same rule
 * parseAmountRounded applies everywhere else — so typing a price of 45,175.004
 * on a 2-decimal quote gives 45,175.00 rather than an error. The form settles
 * the field to the parsed value on blur, so what is on screen is always what
 * would be sent.
 */
export function buildTicket(
  ref: ReferenceData,
  market: Market,
  side: Side,
  priceInput: string,
  quantityInput: string
): { ticket: LimitTicket } | { error: TicketError } {
  const price = parseAmountRounded(priceInput, effectiveExponent(ref, market.quote));
  if (!price.ok) {
    return {
      error: {
        field: "price",
        message:
          price.reason === "negative"
            ? "A limit price cannot be negative."
            : "Enter a limit price."
      }
    };
  }
  if (price.value <= 0n) {
    return { error: { field: "price", message: "The limit price must be above zero." } };
  }

  const quantity = parseAmountRounded(quantityInput, effectiveExponent(ref, market.base));
  if (!quantity.ok) {
    return {
      error: {
        field: "quantity",
        message:
          quantity.reason === "negative" ? "An amount cannot be negative." : "Enter an amount."
      }
    };
  }
  if (quantity.value <= 0n) {
    return { error: { field: "quantity", message: "The amount must be above zero." } };
  }

  return { ticket: { market, side, quantity: quantity.value, price: price.value } };
}
