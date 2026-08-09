import { API_BASE } from "./api";

/**
 * Reference data — the currencies and markets the exchange supports.
 *
 * The backend is the authority. `GET /currencies` carries the exponent that
 * defines what a minor unit means for each currency, and `GET /markets` carries
 * the tradeable pairs and which side of each is base.
 *
 * There is deliberately NO hardcoded copy here any more. One existed while
 * those endpoints were unimplemented; keeping it now would make the frontend a
 * second source of truth for currency precision, and the two silently
 * disagreeing is a factor-of-10^n error in every amount the app displays and
 * sends. Failing loudly is the cheaper outcome, so this module throws rather
 * than substituting values of its own.
 */

export type Currency = {
  code: string;
  name: string;
  /** Decimal places. Minor units per whole unit = 10^exponent. */
  exponent: number;
};

export type Market = {
  symbol: string;
  /** Order quantity is denominated in this currency. */
  base: string;
  /** Price is expressed in this currency. */
  quote: string;
};

export type ReferenceData = {
  currencies: Record<string, Currency>;
  markets: Market[];
};

/**
 * Thrown when reference data cannot be loaded, or loads but is unusable.
 *
 * "Unusable" is a real case rather than a defensive one: entries failing
 * validation are dropped, so a malformed payload can parse to an empty table.
 * Without exponents the app cannot render a balance or build an order, so there
 * is nothing to degrade to.
 */
export class ReferenceUnavailableError extends Error {
  constructor(message: string) {
    super(message);
    this.name = "ReferenceUnavailableError";
  }
}

type CurrencyPayload = { code?: string; name?: string; exponent?: number };
type MarketPayload = { symbol?: string; base?: string; quote?: string };

/** Loads currencies and markets in parallel. Throws if either is unusable. */
export async function loadReference(): Promise<ReferenceData> {
  let currencyRes: Response;
  let marketRes: Response;

  try {
    [currencyRes, marketRes] = await Promise.all([
      fetch(`${API_BASE}/currencies`),
      fetch(`${API_BASE}/markets`)
    ]);
  } catch {
    // fetch only rejects on network-level failure; HTTP errors resolve.
    throw new ReferenceUnavailableError(
      `Cannot reach the API at ${API_BASE}. Is the backend running?`
    );
  }

  if (!currencyRes.ok) {
    throw new ReferenceUnavailableError(`GET /currencies returned ${currencyRes.status}.`);
  }
  if (!marketRes.ok) {
    throw new ReferenceUnavailableError(`GET /markets returned ${marketRes.status}.`);
  }

  let rawCurrencies: unknown;
  let rawMarkets: unknown;
  try {
    [rawCurrencies, rawMarkets] = await Promise.all([currencyRes.json(), marketRes.json()]);
  } catch {
    throw new ReferenceUnavailableError("Reference data was not valid JSON.");
  }

  const currencies = parseCurrencies(rawCurrencies);
  const markets = parseMarkets(rawMarkets, currencies);

  if (Object.keys(currencies).length === 0) {
    throw new ReferenceUnavailableError(
      "GET /currencies returned no usable entries. Each needs a string `code` and an " +
        "integer `exponent` between 0 and 18."
    );
  }
  if (markets.length === 0) {
    throw new ReferenceUnavailableError(
      "GET /markets returned no usable entries. Each needs a string `symbol`, `base` and " +
        "`quote`, and both currencies must appear in GET /currencies."
    );
  }

  return { currencies, markets };
}

function parseCurrencies(raw: unknown): Record<string, Currency> {
  const list = Array.isArray(raw) ? raw : [];
  const out: Record<string, Currency> = {};

  for (const item of list as CurrencyPayload[]) {
    if (typeof item?.code !== "string") continue;
    if (typeof item.exponent !== "number" || !Number.isInteger(item.exponent)) continue;
    if (item.exponent < 0 || item.exponent > 18) continue;

    out[item.code] = {
      code: item.code,
      name: typeof item.name === "string" ? item.name : item.code,
      exponent: item.exponent
    };
  }
  return out;
}

/** Drops any market referencing a currency we have no exponent for. */
function parseMarkets(raw: unknown, currencies: Record<string, Currency>): Market[] {
  const list = Array.isArray(raw) ? raw : [];
  const out: Market[] = [];

  for (const item of list as MarketPayload[]) {
    if (typeof item?.symbol !== "string") continue;
    if (typeof item.base !== "string" || typeof item.quote !== "string") continue;
    if (item.base === item.quote) continue;
    if (!currencies[item.base] || !currencies[item.quote]) continue;

    out.push({ symbol: item.symbol, base: item.base, quote: item.quote });
  }
  return out;
}
