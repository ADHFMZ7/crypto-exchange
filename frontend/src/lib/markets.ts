import {
  divCeil,
  divFloor,
  divRoundHalfUp,
  formatMinorUnits,
  parseAmountRounded,
  pow10,
  toAmount
} from "./decimal";
import type { Side } from "../types";
import type { Market, ReferenceData } from "./reference";

export type { Currency, Market, ReferenceData } from "./reference";

// Side is the wire vocabulary, so it is defined once in types and re-exported
// here for the callers that reason about markets rather than payloads.
export type { Side } from "../types";

/**
 * The decimal places a currency holds — minor units per whole unit is 10^this.
 *
 * Reads the same whether the table came from the API or the hardcoded copy in
 * lib/reference. That is the point: the exponent is a property of the currency,
 * not of how we happened to learn about it, so there is no mode in which this
 * app measures money differently.
 *
 * An unknown currency falls back to 0, which treats a value as already being in
 * minor units rather than inventing a scale factor for something we have no
 * exponent for.
 */
export function effectiveExponent(ref: ReferenceData, code: string): number {
  return ref.currencies[code]?.exponent ?? 0;
}

export type MarketMatch = { market: Market; side: Side };

/**
 * Every market that can trade this currency pair.
 *
 * Returns a list rather than one match because a currency pair is not a unique
 * key — the same two currencies can back spot and a perpetual and a dated
 * future. That is exactly why the request carries a symbol instead of a pair,
 * and it means picking between candidates is the client's job.
 */
export function resolveMarkets(ref: ReferenceData, spend: string, receive: string): MarketMatch[] {
  if (spend === receive) return [];

  const matches: MarketMatch[] = [];
  for (const market of ref.markets) {
    // Acquiring the base currency is a buy; giving it up is a sell.
    if (spend === market.quote && receive === market.base) matches.push({ market, side: "buy" });
    else if (spend === market.base && receive === market.quote) {
      matches.push({ market, side: "sell" });
    }
  }
  return matches;
}

/**
 * Resolves a pair to a single market, honouring an explicit symbol when the pair
 * is ambiguous. Falls back to the first listed market, which is the only
 * candidate in the overwhelmingly common case.
 */
export function resolveTrade(
  ref: ReferenceData,
  spend: string,
  receive: string,
  preferSymbol?: string
): MarketMatch | null {
  const matches = resolveMarkets(ref, spend, receive);
  if (matches.length === 0) return null;

  if (preferSymbol) {
    const preferred = matches.find((m) => m.market.symbol === preferSymbol);
    if (preferred) return preferred;
  }
  return matches[0];
}

export function tradeableCurrencies(ref: ReferenceData): string[] {
  const codes = new Set<string>();
  ref.markets.forEach((m) => {
    codes.add(m.base);
    codes.add(m.quote);
  });
  return [...codes].sort();
}

export function counterpartsFor(ref: ReferenceData, code: string): string[] {
  const codes = new Set<string>();
  ref.markets.forEach((m) => {
    if (m.base === code) codes.add(m.quote);
    if (m.quote === code) codes.add(m.base);
  });
  return [...codes].sort();
}

/**
 * Records that an amount was moved to make the order representable.
 *
 * Only ever describes the quote leg: the base amount is the quantity the user
 * actually asked for and is left alone, while the quote amount is derived from
 * the rounded price.
 */
export type Adjustment = {
  currency: string;
  typed: bigint;
  actual: bigint;
  /** True when the typed input itself carried more precision than the currency has. */
  fromPrecision: boolean;
};

export type OrderIntent = {
  market: Market;
  side: Side;
  /** Wire value for `shares`: base quantity in base minor units. */
  quantity: bigint;
  /** Wire value for `price`: quote minor units per ONE WHOLE base unit. */
  price: bigint;
  spendCurrency: string;
  spendMinor: bigint;
  receiveCurrency: string;
  receiveMinor: bigint;
  /** Human-readable limit rate, e.g. "45,000 USD". */
  rateDisplay: string;
  /** Present when the quote leg had to move. */
  adjustment?: Adjustment;
};

export type IntentError = { message: string };

/**
 * Turns "I spend A of X, I receive B of Y" into the (market, side, quantity,
 * price) tuple the matching engine wants.
 *
 * Regardless of direction the base leg becomes `quantity` and the quote leg
 * scaled by it becomes `price`, which is what keeps the two sides symmetric:
 *
 *     price = quote_minor × 10^base_exponent / base_minor
 */
export function buildIntent(
  ref: ReferenceData,
  spendCurrency: string,
  spendInput: string,
  receiveCurrency: string,
  receiveInput: string,
  preferSymbol?: string
): { intent: OrderIntent } | { error: IntentError } {
  if (spendCurrency === receiveCurrency) {
    return { error: { message: "Pick two different currencies." } };
  }

  const resolved = resolveTrade(ref, spendCurrency, receiveCurrency, preferSymbol);
  if (!resolved) {
    return { error: { message: `No market trades ${spendCurrency} against ${receiveCurrency}.` } };
  }

  const spendExp = effectiveExponent(ref, spendCurrency);
  const receiveExp = effectiveExponent(ref, receiveCurrency);

  // Excess precision is rounded away rather than rejected — see parseAmountRounded.
  const spendParsed = parseAmountRounded(spendInput, spendExp);
  if (!spendParsed.ok) {
    return { error: { message: amountError("spend", spendParsed.reason) } };
  }

  const receiveParsed = parseAmountRounded(receiveInput, receiveExp);
  if (!receiveParsed.ok) {
    return { error: { message: amountError("receive", receiveParsed.reason) } };
  }

  if (spendParsed.value <= 0n || receiveParsed.value <= 0n) {
    return { error: { message: "Both amounts must be greater than zero." } };
  }

  const { market, side } = resolved;
  const buying = side === "buy";
  const baseMinor = buying ? receiveParsed.value : spendParsed.value;
  const quoteTyped = buying ? spendParsed.value : receiveParsed.value;
  const baseExp = effectiveExponent(ref, market.base);
  const quoteExp = effectiveExponent(ref, market.quote);
  const baseScale = pow10(baseExp);

  // price = quote_minor × 10^base_exp / base_minor, snapped to the nearest whole
  // quote minor unit. Rounding here rather than rejecting is what lets the user
  // type any two amounts; the wire format takes `price` directly, so there is
  // nothing downstream that needs the original ratio to have been exact.
  const price = divRoundHalfUp(quoteTyped * baseScale, baseMinor);

  if (price <= 0n) {
    return {
      error: {
        message: `That rate rounds to zero ${minorUnitName(market.quote, quoteExp)} per ${market.base}. Increase the ${market.quote} amount or reduce the ${market.base} amount.`
      }
    };
  }

  // Recompute the quote leg from the rounded price, matching what the backend
  // will compute: a buy locks what it may cost (round up), a sell credits what
  // is guaranteed (round down).
  const quoteActual = buying
    ? divCeil(baseMinor * price, baseScale)
    : divFloor(baseMinor * price, baseScale);

  const typedPrecisionLost = buying ? spendParsed.rounded : receiveParsed.rounded;
  const adjustment: Adjustment | undefined =
    quoteActual === quoteTyped && !typedPrecisionLost
      ? undefined
      : {
          currency: market.quote,
          typed: quoteTyped,
          actual: quoteActual,
          fromPrecision: typedPrecisionLost
        };

  return {
    intent: {
      market,
      side,
      quantity: baseMinor,
      price,
      spendCurrency,
      spendMinor: buying ? quoteActual : baseMinor,
      receiveCurrency,
      receiveMinor: buying ? baseMinor : quoteActual,
      rateDisplay: `${formatMinorUnits(price, quoteExp)} ${market.quote}`,
      adjustment
    }
  };
}

function amountError(leg: "spend" | "receive", reason: "malformed" | "negative"): string {
  if (reason === "negative") return `The ${leg} amount can't be negative.`;
  return `Enter a valid ${leg} amount.`;
}

/** What one minor unit of a currency is called, for user-facing copy. */
export function minorUnitName(code: string, exponent: number): string {
  if (exponent === 0) return `whole ${code}`;
  if (code === "USD") return "cents";
  if (code === "BTC") return "satoshis";
  return `${code} minor units`;
}

/**
 * Builds the POST /orders body.
 *
 * The market symbol is resolved here, from the currency pair the user picked —
 * they never select a ticker. This is the single place the wire format is
 * constructed.
 *
 * All four fields are always present. `side` and order type are independent
 * axes, which is why the old combined `limit_buy` encoding is gone: adding
 * market or stop orders now adds values to a `type` field rather than
 * multiplying out one.
 *
 * TODO(docs/api-todos.md § 1c): amounts become strings when minor-unit values
 * outgrow the IEEE-754 safe range — satoshis fit today, an 18-decimal currency
 * would not. Nothing outside this function changes when they do.
 */
export function toOrderPayload(intent: OrderIntent): {
  market: string;
  side: Side;
  quantity: number;
  price: number;
} {
  return {
    market: intent.market.symbol,
    side: intent.side,
    quantity: Number(intent.quantity),
    price: Number(intent.price)
  };
}

/** Formats an amount already in minor units. */
export function formatAmount(ref: ReferenceData, minor: bigint, code: string): string {
  return `${formatMinorUnits(minor, effectiveExponent(ref, code))} ${code}`;
}

/** Formats a raw balance from GET /wallets/me. */
export function formatBalance(
  ref: ReferenceData,
  available: string | number,
  code: string
): string {
  return formatAmount(ref, toAmount(available), code);
}

/**
 * Renders a submitted order's two amounts back into money.
 *
 * An order records what actually went on the wire — `shares` in base minor
 * units, `price` in quote minor units per whole base — so reading it back needs
 * the same two exponents that produced it, and they come from different
 * currencies. Getting that pairing backwards is silent: both are plausible
 * integers, and the only symptom is a price off by a factor of 10^6.
 *
 * A market with no reference entry has no exponent to apply, so the raw integer
 * is shown rather than a guess at one.
 */
export function formatOrderLegs(
  ref: ReferenceData,
  order: { market: string; quantity: number; filled_quantity?: number; price_each: number }
): { quantity: string; filled: string; price: string } {
  const market = ref.markets.find((m) => m.symbol === order.market);

  const render = (value: number | undefined, code: string | undefined): string => {
    if (value === undefined) return "—";
    // Defensive against a malformed payload: a non-integer must not throw
    // inside BigInt() and take the page down with it.
    if (!Number.isInteger(value)) return String(value);
    if (!code) return value.toLocaleString();
    return formatAmount(ref, BigInt(value), code);
  };

  return {
    quantity: render(order.quantity, market?.base),
    filled: render(order.filled_quantity, market?.base),
    price: render(order.price_each, market?.quote)
  };
}

/**
 * What fraction of an order has filled, 0..1.
 *
 * Reads 0 for everything until the matching engine reports fills — which is
 * accurate rather than a placeholder, since nothing can fill yet.
 */
export function fillFraction(order: { quantity: number; filled_quantity: number }): number {
  if (!order.quantity) return 0;
  return Math.min(Math.max(order.filled_quantity / order.quantity, 0), 1);
}
