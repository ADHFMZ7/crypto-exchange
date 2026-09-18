import type { SourceKind } from "../components/DataSource";

/**
 * The contract between this frontend and the Go backend, written down.
 * Rendered by the Integration panel so the gap is visible in the running app
 * rather than buried in a README.
 */

export type EndpointStatus = {
  method: string;
  path: string;
  /** live = implemented and called by this app. mock = frontend fakes it. */
  state: Extract<SourceKind, "live" | "mock">;
  purpose: string;
  /** What the frontend does in the meantime, for endpoints that do not exist. */
  workaround?: string;
};

export const ENDPOINTS: EndpointStatus[] = [
  {
    method: "POST",
    path: "/users",
    state: "live",
    purpose: "Register an account and seed its starting balance"
  },
  {
    method: "POST",
    path: "/auth/login",
    state: "live",
    purpose: "Exchange credentials for a JWT"
  },
  {
    method: "GET",
    path: "/users/me",
    state: "live",
    purpose: "Identify the signed-in user"
  },
  {
    method: "GET",
    path: "/wallets/me",
    state: "live",
    purpose: "Read available balance per currency"
  },
  {
    method: "PATCH",
    path: "/wallets/me",
    state: "live",
    purpose: "Apply a signed delta to the USD balance"
  },
  {
    method: "POST",
    path: "/orders",
    state: "live",
    purpose: "Place a limit order: market, side, quantity, price"
  },
  {
    method: "GET",
    path: "/currencies",
    state: "live",
    purpose: "Currency codes, names and decimal exponents"
  },
  {
    method: "GET",
    path: "/markets",
    state: "live",
    purpose: "Market list with base/quote roles"
  },
  {
    method: "GET",
    path: "/orders",
    state: "live",
    purpose: "This user's orders, with status and fill progress"
  },
  {
    method: "DELETE",
    path: "/orders/{id}",
    state: "live",
    purpose: "Cancel a resting order and return its unspent lock"
  },
  {
    method: "GET",
    path: "/trades",
    state: "live",
    purpose: "Executions against this user's orders, with price, time and maker/taker role"
  },
  {
    method: "GET",
    path: "/markets/tickers",
    state: "live",
    purpose: "Quote board — last, open, change, high, low and volume for every market"
  },
  {
    method: "GET",
    path: "/markets/{symbol}/ticker",
    state: "live",
    purpose: "One market's trailing-window summary"
  },
  {
    method: "GET",
    path: "/markets/{symbol}/trades",
    state: "live",
    purpose: "Public tape — recent executions, with the side that crossed"
  },
  {
    method: "GET",
    path: "/orderbook/{symbol}",
    state: "live",
    purpose: "Resting bids and asks from the in-memory book"
  },
  {
    method: "GET",
    path: "/markets/{symbol}/candles",
    state: "mock",
    purpose: "Price history for the home page chart",
    workaround: "Chart draws a synthetic random walk"
  }
];

export const LIVE_ENDPOINT_COUNT = ENDPOINTS.filter((e) => e.state === "live").length;
