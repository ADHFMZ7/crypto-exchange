import type {
  CancelAck,
  MarketTradesResponse,
  OrderAck,
  OrderbookSnapshot,
  OrdersResponse,
  Side,
  TickersResponse,
  TradesResponse,
  User,
  Wallet
} from "../types";

export const API_BASE = import.meta.env.VITE_API_BASE_URL ?? "http://localhost:8080";

export class ApiError extends Error {
  readonly status: number;

  constructor(status: number, message: string) {
    super(message);
    this.name = "ApiError";
    this.status = status;
  }

  get isUnauthorized(): boolean {
    return this.status === 401;
  }
}

/** Thrown when the server could not be reached at all, as opposed to returning an error. */
export class NetworkError extends Error {
  constructor(cause: unknown) {
    super(`Cannot reach the API at ${API_BASE}. Is the backend running?`);
    this.name = "NetworkError";
    this.cause = cause;
  }
}

type RequestOptions = {
  method?: string;
  token?: string | null;
  body?: unknown;
};

async function request<T>(path: string, options: RequestOptions = {}): Promise<T> {
  const { method = "GET", token, body } = options;

  const headers: Record<string, string> = {};
  if (body !== undefined) {
    headers["Content-Type"] = "application/json";
  }
  if (token) {
    headers.Authorization = `Bearer ${token}`;
  }

  let res: Response;
  try {
    res = await fetch(`${API_BASE}${path}`, {
      method,
      headers,
      body: body === undefined ? undefined : JSON.stringify(body)
    });
  } catch (err) {
    // fetch only rejects on network-level failure; HTTP errors resolve normally.
    throw new NetworkError(err);
  }

  if (!res.ok) {
    // The Go handlers use http.Error, so failures are text/plain, not JSON.
    const detail = (await res.text()).trim();
    throw new ApiError(res.status, detail || `${method} ${path} failed with ${res.status}`);
  }

  if (res.status === 204 || res.headers.get("Content-Length") === "0") {
    return undefined as T;
  }

  const text = await res.text();
  if (!text) {
    return undefined as T;
  }
  return JSON.parse(text) as T;
}

/*
 * One function per endpoint the Go backend actually serves today.
 * Anything not listed here does not exist server-side yet — see lib/endpoints.
 *
 * EVERY AMOUNT CROSSING THIS BOUNDARY IS INTEGER MINOR UNITS: cents for USD,
 * satoshis for BTC. Not dollars, not bitcoin, not a decimal string. A caller
 * holding a user-typed "0.1" converts it before it gets here — parseAmountRounded
 * in lib/decimal is the only sanctioned way to do that, and lib/markets is the
 * only place that builds a trade payload.
 *
 * `price` is the single exception to "minor units" as a phrase, because it is a
 * rate rather than an amount: it is quote minor units per ONE WHOLE base unit —
 * cents per whole BTC. The alternative reading, minor per minor, makes a
 * realistic BTC price 0.045 and fractional, which is the exact thing integers
 * are here to eliminate. See docs/api-todos.md § 1c.
 */

export const api = {
  login: (email: string, password: string) =>
    request<{ token: string }>("/auth/login", {
      method: "POST",
      body: { email, password }
    }),

  signup: (email: string, fullname: string, password: string) =>
    request<void>("/users", {
      method: "POST",
      body: { email, fullname, password }
    }),

  getMe: (token: string) => request<User>("/users/me", { token }),

  getWallet: (token: string) => request<Wallet>("/wallets/me", { token }),

  /**
   * PATCH /wallets/me — a signed delta applied to one currency, so a withdrawal
   * is the same call with a negative amount.
   *
   * `minorAmount` is that currency's minor units: cents for USD, satoshis for
   * BTC. Which one it is depends entirely on `currency`, which is why they
   * travel together.
   */
  deposit: (token: string, currency: string, minorAmount: number) =>
    request<void>("/wallets/me", {
      method: "PATCH",
      token,
      body: { currency, amount: minorAmount }
    }),

  /**
   * POST /orders — `quantity` is base minor units, `price` is quote minor units
   * per whole base. Build the payload with toOrderPayload rather than by hand;
   * it is the only place the wire format is constructed.
   *
   * All four fields are required. Cancellation is not part of this payload —
   * it is a separate resource, cancelOrder below.
   */
  createOrder: (
    token: string,
    payload: { market: string; side: Side; quantity: number; price: number }
  ) => request<OrderAck>("/orders", { method: "POST", token, body: payload }),

  /** GET /orders — the caller's own orders, newest first. */
  getOrders: (token: string) => request<OrdersResponse>("/orders", { token }),

  /**
   * DELETE /orders/{id} — stop matching a resting order and release its lock.
   *
   * Answers 202, not 200. The book is owned by a worker goroutine and the funds
   * move behind it, so this means the cancellation is queued, not that it has
   * happened. Re-read GET /orders for the outcome.
   *
   * The order can still fill in that window, in which case it ends up `filled`
   * and the cancellation simply lost. That is a normal race, not an error to
   * retry: a second attempt answers 409.
   */
  cancelOrder: (token: string, orderID: number) =>
    request<CancelAck>(`/orders/${orderID}`, { method: "DELETE", token }),

  /**
   * GET /trades — the caller's own executions, newest first.
   *
   * This is the detail behind `filled_quantity` on GET /orders: that says how
   * much of an order filled, this says which trades did it and at what price.
   * An order that walked several price levels has one entry per level.
   */
  getTrades: (token: string, limit?: number) =>
    request<TradesResponse>(`/trades${limit ? `?limit=${limit}` : ""}`, { token }),

  /*
   * The three below are public: no token, because none of them carries anything
   * belonging to an account. Depth has no owners and the tape has no order ids.
   */

  /** GET /markets/tickers — every listed market's trailing-window summary. */
  getTickers: () => request<TickersResponse>("/markets/tickers"),

  /** GET /markets/{symbol}/trades — the public tape, newest first. */
  getMarketTrades: (symbol: string, limit?: number) =>
    request<MarketTradesResponse>(
      `/markets/${encodeURIComponent(symbol)}/trades${limit ? `?limit=${limit}` : ""}`
    ),

  /**
   * GET /orderbook/{symbol} — resting depth, best first on both sides.
   *
   * `limit` caps price levels per side, not orders.
   */
  getOrderbook: (symbol: string, limit?: number) =>
    request<OrderbookSnapshot>(
      `/orderbook/${encodeURIComponent(symbol)}${limit ? `?limit=${limit}` : ""}`
    )
};

/** Turns any thrown value into something safe to render. */
export function errorMessage(err: unknown): string {
  if (err instanceof ApiError || err instanceof NetworkError) {
    return err.message;
  }
  if (err instanceof Error) {
    return err.message;
  }
  return "Something went wrong";
}
