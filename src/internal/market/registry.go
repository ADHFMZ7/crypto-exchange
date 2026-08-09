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
		if c.Exponent < 0 || c.Exponent > 18 {
			return nil, fmt.Errorf("currency %s: exponent %d outside 0..18", c.Code, c.Exponent)
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
func Default() ([]Currency, []Market) {
	usd := Currency{Code: "USD", Name: "US Dollar", Exponent: 2}
	btc := Currency{Code: "BTC", Name: "Bitcoin", Exponent: 8}

	return []Currency{usd, btc},
		[]Market{{Symbol: "BTC-USD", Base: btc, Quote: usd}}
}
