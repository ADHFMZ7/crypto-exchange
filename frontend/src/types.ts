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

export type TradeType = "limit_buy" | "limit_sell" | "cancel";

/** The 202 body returned by POST /trades. */
export type TradeAck = {
  status: string;
  order_id: number;
  market: string;
  type: string;
  receivedAt: string;
};

/**
 * An order this browser submitted. The backend has no GET /orders yet, so the
 * only record of a submission is the ack we keep client-side. See lib/orderLog.
 */
export type LocalOrder = {
  order_id: number;
  market: string;
  type: TradeType;
  /** Base minor units — satoshis on a BTC-base market. Null for a cancel. */
  shares: number | null;
  /** Quote minor units per ONE WHOLE base unit — cents per whole BTC. */
  price: number | null;
  submittedAt: string;
};

export type MarketTicker = {
  symbol: string;
  price: number;
  change: number;
  volume: number;
};
