export type User = {
  id: number;
  fullname: string;
  email: string;
};

/**
 * Mirrors models.Balance in the Go backend.
 * Migration 000002 replaced the single `balance` column with `available`/`locked`.
 */
export type WalletBalance = {
  id: number;
  user_id: number;
  currency: string;
  /**
   * Minor units. Typed as string | number because integer minor units outgrow
   * the IEEE-754 safe range for high-exponent currencies — see toAmount in
   * lib/decimal. Always read these through toAmount, never as raw numbers.
   */
  available: string | number;
  locked: string | number;
};

export type Wallet = {
  user_id: number;
  balances: WalletBalance[];
};

/** Which side of the base currency an order takes. */
export type Side = "buy" | "sell";

/**
 * Order lifecycle, mirroring the orders_status_valid CHECK in migration 000003.
 *
 * `partially_filled` cannot occur until the matching engine reports fills, so
 * everything currently reads `open` — which is accurate, not a placeholder.
 */
export type OrderStatus = "open" | "partially_filled" | "filled" | "cancelled";

/** The 202 body returned by POST /orders. */
export type OrderAck = {
  status: string;
  order_id: number;
  market: string;
  receivedAt: string;
};

/**
 * One order, as returned by GET /orders. Mirrors models.Order.
 *
 * Amounts are integer minor units: `quantity` and `filled_quantity` in BASE
 * minor units (satoshis on BTC-USD), `price_each` in QUOTE minor units per one
 * WHOLE base unit (cents per whole BTC). Read them through toAmount rather than
 * as raw numbers — see lib/decimal.
 */
export type Order = {
  id: number;
  market: string;
  side: Side;
  quantity: number;
  filled_quantity: number;
  price_each: number;
  status: OrderStatus;
  created_at: string;
};

/** GET /orders wraps its list so pagination can be added without breaking it. */
export type OrdersResponse = {
  orders: Order[];
};

export type MarketTicker = {
  symbol: string;
  price: number;
  change: number;
  volume: number;
};
