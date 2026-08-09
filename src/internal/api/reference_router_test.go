package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ADHFMZ7/crypto-exchange/internal/market"
)

/*
These assert the wire contract, not the registry.

The frontend parses both payloads in lib/reference.ts and drops anything that
does not match — silently, because loadReference is designed never to throw. A
wrong field name or a nested object where a string belongs is therefore
indistinguishable from the endpoint not existing: no console error, no failed
request, the UI just keeps showing hardcoded data. That failure mode is why the
shape is pinned here rather than eyeballed.
*/

func referenceRouter(t *testing.T) *ReferenceRouter {
	t.Helper()
	return NewReferenceRouter(minimalServices(t))
}

func TestGetCurrenciesWireShape(t *testing.T) {
	rec := httptest.NewRecorder()
	referenceRouter(t).GetCurrencies(rec, httptest.NewRequest(http.MethodGet, "/currencies", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", ct)
	}

	// Decoded into the client's view of the payload: a top-level array whose
	// exponent is a JSON number, not a string.
	var got []struct {
		Code     string `json:"code"`
		Name     string `json:"name"`
		Exponent *int   `json:"exponent"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("payload is not a JSON array of currencies: %v\nbody: %s", err, rec.Body)
	}

	byCode := map[string]int{}
	for _, c := range got {
		if c.Code == "" {
			t.Fatalf("currency with empty code in %s", rec.Body)
		}
		if c.Exponent == nil {
			t.Fatalf("currency %s has no exponent; the client drops these", c.Code)
		}
		if *c.Exponent < 0 || *c.Exponent > 18 {
			t.Fatalf("currency %s exponent %d outside 0..18", c.Code, *c.Exponent)
		}
		byCode[c.Code] = *c.Exponent
	}

	// The two exponents the whole minor-unit scheme rests on.
	if byCode["USD"] != 2 {
		t.Fatalf("USD exponent = %d, want 2 (cents)", byCode["USD"])
	}
	if byCode["BTC"] != 8 {
		t.Fatalf("BTC exponent = %d, want 8 (satoshis)", byCode["BTC"])
	}
}

func TestGetMarketsWireShape(t *testing.T) {
	rec := httptest.NewRecorder()
	referenceRouter(t).GetMarkets(rec, httptest.NewRequest(http.MethodGet, "/markets", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	// base and quote are strings here. Encoding market.Market directly would
	// nest a whole Currency object in each, and every entry would be dropped.
	var got []struct {
		Symbol string `json:"symbol"`
		Base   string `json:"base"`
		Quote  string `json:"quote"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("payload is not a JSON array of markets: %v\nbody: %s", err, rec.Body)
	}

	if len(got) == 0 {
		t.Fatalf("no markets returned; the client discards the whole response: %s", rec.Body)
	}

	for _, m := range got {
		if m.Symbol == "" || m.Base == "" || m.Quote == "" {
			t.Fatalf("market %+v has an empty field; the client drops these", m)
		}
		if m.Base == m.Quote {
			t.Fatalf("market %s has base == quote", m.Symbol)
		}
	}

	if got[0].Symbol != "BTC-USD" || got[0].Base != "BTC" || got[0].Quote != "USD" {
		t.Fatalf("first market = %+v, want BTC-USD base BTC quote USD", got[0])
	}
}

// Every market must reference a currency that /currencies lists. The client
// drops any market that fails this, and an empty market list makes it discard
// both payloads and fall back — so a mismatch here disables reference data
// entirely rather than degrading it.
func TestMarketsOnlyReferenceListedCurrencies(t *testing.T) {
	router := referenceRouter(t)

	currencyRec := httptest.NewRecorder()
	router.GetCurrencies(currencyRec, httptest.NewRequest(http.MethodGet, "/currencies", nil))

	marketRec := httptest.NewRecorder()
	router.GetMarkets(marketRec, httptest.NewRequest(http.MethodGet, "/markets", nil))

	var currencies []struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(currencyRec.Body.Bytes(), &currencies); err != nil {
		t.Fatal(err)
	}
	var markets []struct {
		Symbol string `json:"symbol"`
		Base   string `json:"base"`
		Quote  string `json:"quote"`
	}
	if err := json.Unmarshal(marketRec.Body.Bytes(), &markets); err != nil {
		t.Fatal(err)
	}

	listed := map[string]bool{}
	for _, c := range currencies {
		listed[c.Code] = true
	}

	for _, m := range markets {
		if !listed[m.Base] {
			t.Fatalf("market %s references base %s, absent from /currencies", m.Symbol, m.Base)
		}
		if !listed[m.Quote] {
			t.Fatalf("market %s references quote %s, absent from /currencies", m.Symbol, m.Quote)
		}
	}
}

// An empty listing has to encode as [] — Registry.Markets() returns a nil slice
// when nothing is listed, and a nil slice marshals to `null`, which the client
// cannot read as an array.
func TestEmptyListingsEncodeAsArraysNotNull(t *testing.T) {
	empty, err := market.NewMarketRegistry(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	router := &ReferenceRouter{Registry: empty}

	rec := httptest.NewRecorder()
	router.GetCurrencies(rec, httptest.NewRequest(http.MethodGet, "/currencies", nil))
	if body := rec.Body.String(); body != "[]\n" {
		t.Fatalf("empty currencies encoded as %q, want \"[]\"", body)
	}
}
