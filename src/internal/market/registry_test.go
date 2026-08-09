package market

import "testing"

func usd() Currency { return Currency{Code: "USD", Name: "US Dollar", Exponent: 2} }
func btc() Currency { return Currency{Code: "BTC", Name: "Bitcoin", Exponent: 8} }
func sol() Currency { return Currency{Code: "SOL", Name: "Solana", Exponent: 9} }

func TestNewMarketRegistryAcceptsDefault(t *testing.T) {
	currencies, markets := Default()

	reg, err := NewMarketRegistry(currencies, markets)
	if err != nil {
		t.Fatalf("Default() should be a valid listing: %v", err)
	}

	m, ok := reg.BySymbol("BTC-USD")
	if !ok {
		t.Fatal("BTC-USD not found by Symbol")
	}
	if m.Base.Code != "BTC" || m.Quote.Code != "USD" {
		t.Fatalf("got base %s quote %s, want BTC/USD", m.Base.Code, m.Quote.Code)
	}
	if m.Base.Exponent != 8 {
		t.Fatalf("base exponent = %d, want 8", m.Base.Exponent)
	}
}

func TestByPairIsOrderIndependent(t *testing.T) {
	reg, err := NewMarketRegistry(Default())
	if err != nil {
		t.Fatal(err)
	}

	forward := reg.ByPair("BTC", "USD")
	reverse := reg.ByPair("USD", "BTC")

	if len(forward) != 1 || len(reverse) != 1 {
		t.Fatalf("got %d forward and %d reverse, want 1 each", len(forward), len(reverse))
	}
	if forward[0].Symbol != reverse[0].Symbol {
		t.Fatalf("%s != %s: pair lookup should not depend on argument order",
			forward[0].Symbol, reverse[0].Symbol)
	}
	if got := reg.ByPair("BTC", "SOL"); got != nil {
		t.Fatalf("unlisted pair returned %v, want nil", got)
	}
}

// A currency carrying only a code must come back with the registry's exponent,
// otherwise every amount computed from it is silently off by a power of ten.
func TestExponentsAreResolvedFromTheCurrencyTable(t *testing.T) {
	bareBTC := Currency{Code: "BTC"}
	bareUSD := Currency{Code: "USD"}

	reg, err := NewMarketRegistry(
		[]Currency{usd(), btc()},
		[]Market{{Symbol: "BTC-USD", Base: bareBTC, Quote: bareUSD}},
	)
	if err != nil {
		t.Fatal(err)
	}

	m, _ := reg.BySymbol("BTC-USD")
	if m.Base.Exponent != 8 || m.Quote.Exponent != 2 {
		t.Fatalf("got base %d quote %d, want 8 and 2", m.Base.Exponent, m.Quote.Exponent)
	}
}

func TestSeveralMarketsMayShareAPair(t *testing.T) {
	u, b := usd(), btc()

	reg, err := NewMarketRegistry(
		[]Currency{u, b},
		[]Market{
			{Symbol: "BTC-USD", Base: b, Quote: u},
			{Symbol: "BTC-USD-PERP", Base: b, Quote: u},
		},
	)
	if err != nil {
		t.Fatalf("same orientation twice is legitimate: %v", err)
	}

	got := reg.ByPair("BTC", "USD")
	if len(got) != 2 {
		t.Fatalf("got %d markets, want 2", len(got))
	}
	// Listing order decides the default when a client takes the first match.
	if got[0].Symbol != "BTC-USD" {
		t.Fatalf("first match = %s, want BTC-USD (listing order)", got[0].Symbol)
	}
}

func TestRejects(t *testing.T) {
	u, b, s := usd(), btc(), sol()

	cases := []struct {
		name       string
		currencies []Currency
		markets    []Market
	}{
		{
			name:       "unknown base currency",
			currencies: []Currency{u},
			markets:    []Market{{Symbol: "BTC-USD", Base: b, Quote: u}},
		},
		{
			name:       "base equals quote",
			currencies: []Currency{u},
			markets:    []Market{{Symbol: "USD-USD", Base: u, Quote: u}},
		},
		{
			name:       "duplicate Symbol",
			currencies: []Currency{u, b},
			markets: []Market{
				{Symbol: "BTC-USD", Base: b, Quote: u},
				{Symbol: "BTC-USD", Base: b, Quote: u},
			},
		},
		{
			name:       "duplicate currency",
			currencies: []Currency{u, u},
			markets:    nil,
		},
		{
			name:       "exponent out of range",
			currencies: []Currency{{Code: "XXX", Exponent: 19}},
			markets:    nil,
		},
		{
			name:       "empty currency code",
			currencies: []Currency{{Code: "", Exponent: 2}},
			markets:    nil,
		},
		{
			name:       "empty Symbol",
			currencies: []Currency{u, b},
			markets:    []Market{{Symbol: "", Base: b, Quote: u}},
		},
		{
			// Two books for the same thing at reciprocal prices.
			name:       "both orientations of a pair",
			currencies: []Currency{b, s},
			markets: []Market{
				{Symbol: "SOL-BTC", Base: s, Quote: b},
				{Symbol: "BTC-SOL", Base: b, Quote: s},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := NewMarketRegistry(tc.currencies, tc.markets); err == nil {
				t.Fatal("expected an error, got nil")
			}
		})
	}
}

// The registry is shared by every request goroutine, so a caller must not be
// able to reach into it through a returned slice.
func TestReturnedSlicesAreCopies(t *testing.T) {
	reg, err := NewMarketRegistry(Default())
	if err != nil {
		t.Fatal(err)
	}

	reg.Markets()[0].Symbol = "TAMPERED"
	reg.ByPair("BTC", "USD")[0].Symbol = "TAMPERED"

	if m := reg.Markets(); m[0].Symbol != "BTC-USD" {
		t.Fatalf("registry mutated through a returned slice: %s", m[0].Symbol)
	}
}

func TestCurrenciesAreSortedForStableOutput(t *testing.T) {
	reg, err := NewMarketRegistry([]Currency{sol(), usd(), btc()}, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Map iteration is randomised; GET /currencies must not reshuffle per call.
	for i := 0; i < 20; i++ {
		got := reg.Currencies()
		if len(got) != 3 || got[0].Code != "BTC" || got[1].Code != "SOL" || got[2].Code != "USD" {
			t.Fatalf("unstable or unsorted output: %+v", got)
		}
	}
}
