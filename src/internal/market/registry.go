package market

import (
	"fmt"
	"sort"
)

// Registry is the set of currencies and markets the exchange supports.
//
// It is built once at startup and never mutated afterwards, so it is safe for
// concurrent reads from every request goroutine without locking. If markets ever
// need to change while the process is running, build a fresh Registry and swap
// an atomic.Pointer[Registry] rather than adding a mutex here.
type Registry struct {
	currencies map[string]Currency
	bySymbol   map[string]Market
	byPair     map[pairKey][]Market

	// ordered preserves listing order. Go randomises map iteration, so anything
	// returning a list must range this slice — never one of the maps — or the
	// answer changes between calls and callers that take the first match get
	// non-deterministic behaviour.
	ordered []Market
}

// MaxExponent is the largest number of decimal places a currency may declare.
//
// Nine, not eighteen, and the wire format is why. Amounts cross as JSON
// numbers, which a client parses as a float64: integers stay exact only below
// 2^53, about 9.007e15. At exponent 9 that still leaves nine million whole
// units exact, which is room enough. At 18 — Ethereum's wei — one whole unit is
// 1e18, a hundred times past the ceiling, so a client could not represent a
// single coin without silently rounding it.
//
// int64 gives out at almost the same place: 1e18 wei per coin leaves room for
// nine of them in the largest integer the schema can hold.
//
// This is a property of the current wire format, not of money. When amounts
// become strings (docs/api-todos.md § 1c) the JSON ceiling disappears and this
// bound can rise to whatever int64 allows.
const MaxExponent = 9

// pairKey is order-independent, so (BTC,USD) and (USD,BTC) hit one bucket.
type pairKey [2]string

func newPairKey(a, b string) pairKey {
	if a < b {
		return pairKey{a, b}
	}
	return pairKey{b, a}
}

// NewMarketRegistry validates the given currencies and markets, then indexes
// them for lookup.
//
// Every error it returns describes a misconfiguration that cannot be usefully
// recovered from at request time, so callers should treat failure as fatal
// during startup rather than degrading.
func NewMarketRegistry(currencies []Currency, markets []Market) (*Registry, error) {
	r := &Registry{
		currencies: make(map[string]Currency, len(currencies)),
		bySymbol:   make(map[string]Market, len(markets)),
		byPair:     make(map[pairKey][]Market, len(markets)),
		ordered:    make([]Market, 0, len(markets)),
	}

	for i := range currencies {
		c := currencies[i]

		if c.Code == "" {
			return nil, fmt.Errorf("currency at index %d has an empty code", i)
		}
		if c.Exponent < 0 || c.Exponent > MaxExponent {
			return nil, fmt.Errorf("currency %s: exponent %d outside 0..%d",
				c.Code, c.Exponent, MaxExponent)
		}
		if _, dup := r.currencies[c.Code]; dup {
			return nil, fmt.Errorf("duplicate currency %s", c.Code)
		}

		r.currencies[c.Code] = c
	}

	for i, m := range markets {
		if m.Symbol == "" {
			return nil, fmt.Errorf("market at index %d has an empty Symbol", i)
		}
		if _, dup := r.bySymbol[m.Symbol]; dup {
			return nil, fmt.Errorf("duplicate market Symbol %s", m.Symbol)
		}
		base, ok := r.currencies[m.Base.Code]
		if !ok {
			return nil, fmt.Errorf("market %s: unknown base currency %q", m.Symbol, m.Base.Code)
		}
		quote, ok := r.currencies[m.Quote.Code]
		if !ok {
			return nil, fmt.Errorf("market %s: unknown quote currency %q", m.Symbol, m.Quote.Code)
		}
		if base.Code == quote.Code {
			return nil, fmt.Errorf("market %s: base and quote are both %s", m.Symbol, base.Code)
		}

		// Re-point at the registry's own records so exponents are authoritative
		// even when the caller supplied a Currency carrying only a code.
		m.Base, m.Quote = base, quote

		key := newPairKey(base.Code, quote.Code)

		// Several markets may share a pair (spot alongside a perpetual), but they
		// must agree on which currency is base. Listing both orientations would
		// mean two books for the same thing at reciprocal prices, and side
		// derivation would have no single answer.
		if prior := r.byPair[key]; len(prior) > 0 && prior[0].Base.Code != base.Code {
			return nil, fmt.Errorf(
				"market %s inverts %s: both orientations of %s/%s are listed",
				m.Symbol, prior[0].Symbol, base.Code, quote.Code,
			)
		}

		r.bySymbol[m.Symbol] = m
		r.byPair[key] = append(r.byPair[key], m)
		r.ordered = append(r.ordered, m)
	}

	return r, nil
}

// BySymbol looks a market up by its symbol, e.g. "BTC-USD".
func (r *Registry) BySymbol(Symbol string) (Market, bool) {
	m, ok := r.bySymbol[Symbol]
	return m, ok
}

// ByPair returns every market trading the two currencies, in listing order.
//
// A currency pair is not a unique key, so this can return more than one and the
// caller has to choose. That is precisely why an order carries a Symbol rather
// than a pair.
func (r *Registry) ByPair(a, b string) []Market {
	found := r.byPair[newPairKey(a, b)]
	if len(found) == 0 {
		return nil
	}
	return append([]Market(nil), found...)
}

// Currency looks a currency up by code.
func (r *Registry) Currency(code string) (Currency, bool) {
	c, ok := r.currencies[code]
	return c, ok
}

// Markets returns every listed market in listing order, for GET /markets.
func (r *Registry) Markets() []Market {
	return append([]Market(nil), r.ordered...)
}

// Currencies returns every known currency sorted by code, for GET /currencies.
func (r *Registry) Currencies() []Currency {
	out := make([]Currency, 0, len(r.currencies))
	for _, c := range r.currencies {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Code < out[j].Code })
	return out
}

// Default is the hardcoded listing, standing in until the currencies and markets
// tables exist. It replaces the `symbols` slice previously hardcoded in
// NewTradeService — see docs/design/markets-and-currencies.md.
// Default is the listing the exchange starts with.
//
// Exponents are trading precision, not each chain's native unit. BTC and SOL
// happen to match theirs — satoshis and lamports — but ETH does not: wei is 18
// decimals, which MaxExponent forbids for reasons the wire format decides
// rather than Ethereum. Eight decimals is what an exchange would quote ETH to
// anyway, so nothing here pretends to be a wallet.
//
// Listing order is the order these are returned in, and the frontend picks the
// first market as its default, so BTC-USD stays at the front.
func Default() ([]Currency, []Market) {
	usd := Currency{Code: "USD", Name: "US Dollar", Exponent: 2}
	btc := Currency{Code: "BTC", Name: "Bitcoin", Exponent: 8}
	eth := Currency{Code: "ETH", Name: "Ethereum", Exponent: 8}
	sol := Currency{Code: "SOL", Name: "Solana", Exponent: 9}

	return []Currency{usd, btc, eth, sol},
		[]Market{
			{Symbol: "BTC-USD", Base: btc, Quote: usd},
			{Symbol: "ETH-USD", Base: eth, Quote: usd},
			{Symbol: "SOL-USD", Base: sol, Quote: usd},
		}
}
