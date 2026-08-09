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
    state: "mock",
    purpose: "Cancel a resting order",
    workaround: "No cancel UI — the trade form only places orders"
  },
  {
    method: "GET",
    path: "/markets/{symbol}/ticker",
    state: "mock",
    purpose: "Quote board — last price, 24h change, volume",
    workaround: "Ticker tables render a hardcoded list with a random walk"
  },
  {
    method: "GET",
    path: "/markets/{symbol}/candles",
    state: "mock",
    purpose: "Price history for the home page chart",
    workaround: "Chart draws a synthetic random walk"
  },
  {
    method: "GET",
    path: "/orderbook/{symbol}",
    state: "mock",
    purpose: "Live bids and asks from the in-memory book",
    workaround: "Not surfaced anywhere in the UI yet"
  }
];

export const LIVE_ENDPOINT_COUNT = ENDPOINTS.filter((e) => e.state === "live").length;
