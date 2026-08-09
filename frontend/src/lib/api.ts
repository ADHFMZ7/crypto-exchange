import type { TradeAck, TradeType, User, Wallet } from "../types";

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
   * PATCH /wallets/me — a signed delta in USD minor units, so 500 is five
   * dollars and a withdrawal is negative. The balances table is already
   * denominated in cents (users.GiveBalance seeds 1_000_000 for $10,000), so
   * cents is what keeps a deposit consistent with the balance it lands in.
   */
  deposit: (token: string, minorAmount: number) =>
    request<void>("/wallets/me", {
      method: "PATCH",
      token,
      body: { Amount: minorAmount }
    }),

  /**
   * POST /trades — `shares` is base minor units, `price` is quote minor units
   * per whole base. Build the payload with toTradePayload rather than by hand.
   */
  submitTrade: (
    token: string,
    payload: { market: string; type: TradeType; shares?: number; price?: number; order_id?: number }
  ) => request<TradeAck>("/trades", { method: "POST", token, body: payload })
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
