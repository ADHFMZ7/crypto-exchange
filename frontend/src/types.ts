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
 * Settlement derives this from `filled_quantity` rather than setting it
 * independently, so the two can never disagree. An order reaches
 * `partially_filled` or `filled` without the client doing anything — matching
 * happens after the 202, so poll rather than assuming a fetched value is still
 * current.
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

/**
 * One execution behind an order's fill progress, as GET /trades reports it.
 *
 * `id` is the trade id and is NOT unique in this list: a user who was on both
 * sides of the same trade — nothing prevents self-trading — gets one entry per
 * side. Key rows on `id` and `side` together.
 *
 * `side` is the caller's own side, and `taker` says whether their order was the
 * one that crossed. A taker on a buy paid at most their limit and often less;
 * the difference came back to their available balance when the order closed.
 */
export type Fill = {
  id: number;
  market: string;
  /** The caller's order, not the counterparty's. */
  order_id: number;
  side: Side;
  /** Base minor units. */
  quantity: number;
  /** Quote minor units per one WHOLE base unit. */
  price: number;
  taker: boolean;
  executed_at: string;
};

export type TradesResponse = {
  trades: Fill[];
};

/**
 * One execution on the public tape. No order ids and no owners — this is what
 * GET /markets/{symbol}/trades can serve without authentication.
 *
 * `taker_side` is the direction the aggressor went: a "buy" lifted an offer, a
 * "sell" hit a bid. It is the only thing on the tape that says who was
 * impatient.
 */
export type MarketTrade = {
  id: number;
  market: string;
  quantity: number;
  price: number;
  taker_side: Side;
  executed_at: string;
};

export type MarketTradesResponse = {
  trades: MarketTrade[];
};

/**
 * A market's trailing-window summary, from GET /markets/{symbol}/ticker.
 *
 * Every price is quote minor units per one WHOLE base unit; `base_volume` is
 * base minor units. `change` is absolute rather than a percentage, and
 * `open_price` sits beside it so the client forms the ratio at display time —
 * the server does not pick a precision on the frontend's behalf.
 *
 * `has_traded` distinguishes a market with no history from one that traded at
 * zero. The latter cannot happen today, but reading `last_price === 0` as "no
 * trades" would be inventing a rule the wire does not state.
 */
export type Ticker = {
  market: string;
  has_traded: boolean;
  last_price: number;
  open_price: number;
  change: number;
  high: number;
  low: number;
  base_volume: number;
  trade_count: number;
  window_hours: number;
  last_trade_at: string | null;
};

export type TickersResponse = {
  tickers: Ticker[];
};

/** One price rung of the resting book. */
export type DepthLevel = {
  /** Quote minor units per one WHOLE base unit. */
  price: number;
  /** Base minor units resting at this price. */
  quantity: number;
  /** How many orders make up that quantity. */
  orders: number;
};

/**
 * GET /orderbook/{symbol}. Both sides are best-first: bids descending, asks
 * ascending, so `bids[0]` and `asks[0]` are the touch.
 *
 * This is a snapshot of an in-memory book, correct at the instant the matching
 * worker read it and stale as soon as the next order arrives. Treat it as a
 * picture, never as state to reconcile against.
 */
export type OrderbookSnapshot = {
  market: string;
  bids: DepthLevel[];
  asks: DepthLevel[];
};

/** The 202 body returned by DELETE /orders/{id}. */
export type CancelAck = {
  status: string;
  order_id: number;
};
