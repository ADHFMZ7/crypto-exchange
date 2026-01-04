export type User = {
  id: number;
  fullname: string;
  email: string;
};

export type WalletBalance = {
  currency: string;
  amount: number;
};

export type Trade = {
  id: string;
  market: string;
  side: "buy" | "sell";
  quantity: number;
  price: number;
  status: "open" | "filled" | "cancelled" | "settled";
  placedAt: string;
};

export type MarketTicker = {
  symbol: string;
  price: number;
  change: number;
  volume: number;
};
