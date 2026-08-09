/**
 * Decimal ↔ minor-unit conversion.
 *
 * Money never touches a float here. `0.1 * 1e8` is 10000000.000000002 in IEEE
 *754, which is exactly the class of error that turns into a balance that won't
 * reconcile. Everything below is string parsing into BigInt.
 *
 * "Minor units" means the smallest indivisible unit of a currency: cents for
 * USD (exponent 2), satoshis for BTC (exponent 8).
 */

const DECIMAL_PATTERN = /^\d*(\.\d*)?$/;

export type RoundedParse =
  | { ok: true; value: bigint; rounded: boolean }
  | { ok: false; reason: "malformed" | "negative" };

/**
 * Parse a user-typed decimal string into minor units, rounding half-up on any
 * precision the currency cannot hold rather than rejecting it.
 *
 * "0.1"   at exponent 8 → 10_000_000n, rounded: false
 * "0.005" at exponent 2 → 1n (one cent), rounded: true
 * "0.10"  at exponent 2 → 10n, rounded: false — trailing zeros discard nothing
 * "0.5"   at exponent 0 → 1n, rounded: true
 *
 * `rounded` is true only when a non-zero digit was actually discarded, so the UI
 * can stay quiet about "45.10" while flagging "45.109".
 */
export function parseAmountRounded(input: string, exponent: number): RoundedParse {
  const trimmed = input.trim();

  if (trimmed.startsWith("-")) return { ok: false, reason: "negative" };
  if (trimmed === "" || trimmed === ".") return { ok: false, reason: "malformed" };
  if (!DECIMAL_PATTERN.test(trimmed)) return { ok: false, reason: "malformed" };

  const [whole, fraction = ""] = trimmed.split(".");

  const kept = fraction.slice(0, exponent).padEnd(exponent, "0");
  const dropped = fraction.slice(exponent);

  let value = BigInt((whole || "0") + kept);

  // Round half-up on the first discarded digit.
  if (dropped && Number(dropped.charAt(0)) >= 5) {
    value += 1n;
  }

  return { ok: true, value, rounded: /[1-9]/.test(dropped) };
}

/** Integer division rounding half-up. All inputs must be positive. */
export function divRoundHalfUp(numerator: bigint, denominator: bigint): bigint {
  return (2n * numerator + denominator) / (2n * denominator);
}

/** Integer division rounding away from zero. All inputs must be positive. */
export function divCeil(numerator: bigint, denominator: bigint): bigint {
  return (numerator + denominator - 1n) / denominator;
}

/** Integer division truncating toward zero — floor, for positive inputs. */
export function divFloor(numerator: bigint, denominator: bigint): bigint {
  return numerator / denominator;
}

/** Render minor units back to a plain decimal string, without trailing zeros. */
export function fromMinorUnits(minor: bigint, exponent: number): string {
  const negative = minor < 0n;
  const digits = (negative ? -minor : minor).toString().padStart(exponent + 1, "0");

  const whole = digits.slice(0, digits.length - exponent);
  const fraction = exponent === 0 ? "" : digits.slice(digits.length - exponent).replace(/0+$/, "");

  const rendered = fraction ? `${whole}.${fraction}` : whole;
  return negative ? `-${rendered}` : rendered;
}

/** Group the integer part with locale separators, leaving the fraction intact. */
export function formatMinorUnits(minor: bigint, exponent: number): string {
  const plain = fromMinorUnits(minor, exponent);
  const [whole, fraction] = plain.split(".");
  const grouped = BigInt(whole).toLocaleString();
  return fraction ? `${grouped}.${fraction}` : grouped;
}

/**
 * Coerces a wire amount to BigInt minor units, accepting either a JSON string
 * or a JSON number.
 *
 * Strings are the safe encoding and what docs/api-todos.md specifies: JSON
 * numbers are IEEE-754 doubles, so integers above 2^53 (~9.007 × 10^15) lose
 * precision silently. Satoshis stay under that ceiling, but an 18-decimal
 * currency does not — one whole ETH is 10^18 wei, past it by two orders of
 * magnitude. Reading both keeps this working across that change.
 */
export function toAmount(value: string | number): bigint {
  if (typeof value === "string") {
    const trimmed = value.trim();
    if (!/^-?\d+$/.test(trimmed)) return 0n;
    return BigInt(trimmed);
  }
  if (!Number.isFinite(value)) return 0n;
  return BigInt(Math.trunc(value));
}

/** 10^n as a BigInt, for scaling between exponents. */
export function pow10(n: number): bigint {
  return 10n ** BigInt(n);
}
