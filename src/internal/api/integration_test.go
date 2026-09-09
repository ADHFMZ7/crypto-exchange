package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ADHFMZ7/crypto-exchange/internal/market"
	"github.com/ADHFMZ7/crypto-exchange/internal/models"
	"github.com/ADHFMZ7/crypto-exchange/internal/services"
	"github.com/ADHFMZ7/crypto-exchange/internal/stores"
	"github.com/ADHFMZ7/crypto-exchange/internal/testsupport"
)

/*
The HTTP contract, over a real Postgres.

routers_test.go covers the guards a handler applies before it reaches a service.
These go the other way: a real request, through the real router, against real
stores, asserting the status code and the JSON body a client would actually get.
That is the layer the frontend is written against, so it is the layer where a
changed field name or a wrong status is worth catching.
*/

type api struct {
	t      *testing.T
	router http.Handler
}

func newAPI(t *testing.T) *api {
	t.Helper()

	pool := testsupport.Pool(t, "api")
	testsupport.Truncate(t, pool)

	registry, err := market.NewMarketRegistry(market.Default())
	if err != nil {
		t.Fatal(err)
	}

	// The real wiring, workers and all: matching and settlement run behind the
	// responses exactly as they do in production.
	all := stores.NewStores(pool)
	svc := services.NewServices(all, registry, make(chan models.LedgerEvent, 256))

	return &api{t: t, router: NewRouter(svc)}
}

func (a *api) do(method, path, token string, body any) *httptest.ResponseRecorder {
	a.t.Helper()

	var payload *bytes.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			a.t.Fatal(err)
		}
		payload = bytes.NewReader(encoded)
	} else {
		payload = bytes.NewReader(nil)
	}

	req := httptest.NewRequest(method, path, payload)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	rec := httptest.NewRecorder()
	a.router.ServeHTTP(rec, req)
	return rec
}

// decode reads a JSON body, failing the test if it is not what it claims.
func (a *api) decode(rec *httptest.ResponseRecorder, into any) {
	a.t.Helper()
	if err := json.Unmarshal(rec.Body.Bytes(), into); err != nil {
		a.t.Fatalf("body is not JSON (%d): %s", rec.Code, rec.Body.String())
	}
}

// account registers a user, funds them, and returns their token.
func (a *api) account(email string, usd, btc int64) string {
	a.t.Helper()

	if rec := a.do(http.MethodPost, "/users", "", map[string]string{
		"email": email, "fullname": email, "password": "integration-password",
	}); rec.Code >= 400 {
		a.t.Fatalf("signup %s: %d %s", email, rec.Code, rec.Body.String())
	}

	rec := a.do(http.MethodPost, "/auth/login", "", map[string]string{
		"email": email, "password": "integration-password",
	})
	if rec.Code != http.StatusOK {
		a.t.Fatalf("login %s: %d %s", email, rec.Code, rec.Body.String())
	}
	var issued struct {
		Token string `json:"token"`
	}
	a.decode(rec, &issued)
	if issued.Token == "" {
		a.t.Fatal("login returned no token")
	}

	for currency, amount := range map[string]int64{"USD": usd, "BTC": btc} {
		if amount == 0 {
			continue
		}
		if rec := a.do(http.MethodPatch, "/wallets/me", issued.Token,
			map[string]any{"currency": currency, "amount": amount}); rec.Code >= 400 {
			a.t.Fatalf("funding %s with %s: %d %s", email, currency, rec.Code, rec.Body.String())
		}
	}
	return issued.Token
}

func (a *api) expect(rec *httptest.ResponseRecorder, want int, what string) {
	a.t.Helper()
	if rec.Code != want {
		a.t.Fatalf("%s = %d, want %d — body: %s", what, rec.Code, want, rec.Body.String())
	}
}

// ── accounts ──────────────────────────────────────────────────────────────

func TestSignupAndLogin(t *testing.T) {
	a := newAPI(t)

	a.expect(a.do(http.MethodPost, "/users", "", map[string]string{
		"email": "new@test", "fullname": "New", "password": "integration-password",
	}), http.StatusCreated, "signup")

	// The same address twice is a conflict, not a second account.
	if rec := a.do(http.MethodPost, "/users", "", map[string]string{
		"email": "new@test", "fullname": "New", "password": "integration-password",
	}); rec.Code < 400 {
		t.Errorf("signing up twice = %d, want a failure", rec.Code)
	}

	a.expect(a.do(http.MethodPost, "/auth/login", "", map[string]string{
		"email": "new@test", "password": "wrong",
	}), http.StatusUnauthorized, "login with the wrong password")

	a.expect(a.do(http.MethodPost, "/auth/login", "", map[string]string{
		"email": "nobody@test", "password": "integration-password",
	}), http.StatusUnauthorized, "login as nobody")
}

func TestWalletReadsAndDeposits(t *testing.T) {
	a := newAPI(t)
	token := a.account("wallet@test", 100_000, 0)

	rec := a.do(http.MethodGet, "/wallets/me", token, nil)
	a.expect(rec, http.StatusOK, "GET /wallets/me")

	var wallet models.Wallet
	a.decode(rec, &wallet)
	if len(wallet.Balances) == 0 {
		t.Fatal("wallet has no balances")
	}
	for _, balance := range wallet.Balances {
		if balance.Currency == "USD" && balance.Available < 100_000 {
			t.Errorf("USD available = %d, want at least the deposit", balance.Available)
		}
	}

	a.expect(a.do(http.MethodGet, "/wallets/me", "", nil), http.StatusUnauthorized, "unauthenticated wallet")

	// A withdrawal larger than the balance must not go through.
	if rec := a.do(http.MethodPatch, "/wallets/me", token,
		map[string]any{"currency": "USD", "amount": -999_999_999}); rec.Code < 400 {
		t.Errorf("overdraft = %d, want a failure", rec.Code)
	}
}

// ── orders ────────────────────────────────────────────────────────────────

func TestPlacingAnOrderAnswers202AndReadsBack(t *testing.T) {
	a := newAPI(t)
	token := a.account("trader@test", 10_000_000, 0)

	rec := a.do(http.MethodPost, "/orders", token, map[string]any{
		"market": "BTC-USD", "side": "buy", "quantity": 100_000_000, "price": 4_500_000,
	})
	a.expect(rec, http.StatusAccepted, "POST /orders")

	var ack struct {
		OrderID int64  `json:"order_id"`
		Market  string `json:"market"`
		Status  string `json:"status"`
	}
	a.decode(rec, &ack)
	if ack.OrderID == 0 || ack.Market != "BTC-USD" || ack.Status != "accepted" {
		t.Fatalf("ack = %+v", ack)
	}

	list := a.do(http.MethodGet, "/orders", token, nil)
	a.expect(list, http.StatusOK, "GET /orders")

	var orders models.Orders
	a.decode(list, &orders)
	if len(orders.Orders) != 1 || orders.Orders[0].ID != ack.OrderID {
		t.Fatalf("orders = %+v, want the one just placed", orders.Orders)
	}
	if orders.Orders[0].Status != models.OrderOpen {
		t.Errorf("status = %q, want open", orders.Orders[0].Status)
	}
}

func TestPlacingAnInvalidOrderIsRefused(t *testing.T) {
	a := newAPI(t)
	token := a.account("trader@test", 10_000_000, 0)

	cases := []struct {
		name string
		body map[string]any
	}{
		{"no market", map[string]any{"side": "buy", "quantity": 1, "price": 1}},
		{"unknown market", map[string]any{"market": "NOPE-USD", "side": "buy", "quantity": 1, "price": 1}},
		{"unknown side", map[string]any{"market": "BTC-USD", "side": "up", "quantity": 1, "price": 1}},
		{"zero quantity", map[string]any{"market": "BTC-USD", "side": "buy", "quantity": 0, "price": 1}},
		{"negative price", map[string]any{"market": "BTC-USD", "side": "buy", "quantity": 1, "price": -1}},
		{"unaffordable", map[string]any{
			"market": "BTC-USD", "side": "buy", "quantity": 100_000_000, "price": 99_999_999}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if rec := a.do(http.MethodPost, "/orders", token, tc.body); rec.Code < 400 {
				t.Fatalf("= %d, want a refusal", rec.Code)
			}
		})
	}

	a.expect(a.do(http.MethodPost, "/orders", token, nil), http.StatusBadRequest, "empty body")
	a.expect(a.do(http.MethodPost, "/orders", "", map[string]any{
		"market": "BTC-USD", "side": "buy", "quantity": 1, "price": 1,
	}), http.StatusUnauthorized, "unauthenticated order")
}

func TestCancellingAnOrderOverHTTP(t *testing.T) {
	a := newAPI(t)
	owner := a.account("owner@test", 10_000_000, 0)
	stranger := a.account("stranger@test", 10_000_000, 0)

	rec := a.do(http.MethodPost, "/orders", owner, map[string]any{
		"market": "BTC-USD", "side": "buy", "quantity": 100_000_000, "price": 4_500_000,
	})
	var ack struct {
		OrderID int64 `json:"order_id"`
	}
	a.decode(rec, &ack)

	a.expect(a.do(http.MethodDelete, fmt.Sprintf("/orders/%d", ack.OrderID), "", nil),
		http.StatusUnauthorized, "unauthenticated cancel")
	a.expect(a.do(http.MethodDelete, "/orders/abc", owner, nil),
		http.StatusBadRequest, "cancel with a non-numeric id")
	a.expect(a.do(http.MethodDelete, "/orders/0", owner, nil),
		http.StatusBadRequest, "cancel order zero")
	a.expect(a.do(http.MethodDelete, "/orders/999999", owner, nil),
		http.StatusNotFound, "cancel a missing order")
	a.expect(a.do(http.MethodDelete, fmt.Sprintf("/orders/%d", ack.OrderID), stranger, nil),
		http.StatusNotFound, "cancel another user's order")

	a.expect(a.do(http.MethodDelete, fmt.Sprintf("/orders/%d", ack.OrderID), owner, nil),
		http.StatusAccepted, "cancel")

	// The book and the ledger settle behind the 202.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		list := a.do(http.MethodGet, "/orders", owner, nil)
		var orders models.Orders
		a.decode(list, &orders)
		if len(orders.Orders) == 1 && orders.Orders[0].Status == models.OrderCancelled {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatal("the order never reached cancelled")
}

// ── market data ───────────────────────────────────────────────────────────

func TestMarketDataShapes(t *testing.T) {
	a := newAPI(t)
	seller := a.account("seller@test", 0, 500_000_000)
	buyer := a.account("buyer@test", 100_000_000, 0)

	a.do(http.MethodPost, "/orders", seller, map[string]any{
		"market": "BTC-USD", "side": "sell", "quantity": 100_000_000, "price": 4_500_000})
	a.do(http.MethodPost, "/orders", buyer, map[string]any{
		"market": "BTC-USD", "side": "buy", "quantity": 50_000_000, "price": 4_600_000})

	// Depth and the tape are public: no token anywhere below.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		var tape models.MarketTrades
		a.decode(a.do(http.MethodGet, "/markets/BTC-USD/trades", "", nil), &tape)
		if len(tape.Trades) > 0 {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}

	var tape models.MarketTrades
	rec := a.do(http.MethodGet, "/markets/BTC-USD/trades", "", nil)
	a.expect(rec, http.StatusOK, "GET /markets/{symbol}/trades")
	a.decode(rec, &tape)
	if len(tape.Trades) != 1 {
		t.Fatalf("tape = %+v, want the one execution", tape.Trades)
	}
	if tape.Trades[0].TakerSide != "buy" || tape.Trades[0].Price != 4_500_000 {
		t.Errorf("trade = %+v, want a buy taker at the resting ask", tape.Trades[0])
	}

	var ticker models.Ticker
	rec = a.do(http.MethodGet, "/markets/BTC-USD/ticker", "", nil)
	a.expect(rec, http.StatusOK, "GET /markets/{symbol}/ticker")
	a.decode(rec, &ticker)
	if !ticker.HasTraded || ticker.LastPrice != 4_500_000 || ticker.TradeCount != 1 {
		t.Errorf("ticker = %+v", ticker)
	}

	var tickers struct {
		Tickers []models.Ticker `json:"tickers"`
	}
	rec = a.do(http.MethodGet, "/markets/tickers", "", nil)
	a.expect(rec, http.StatusOK, "GET /markets/tickers")
	a.decode(rec, &tickers)
	_, listedMarkets := market.Default()
	if len(tickers.Tickers) != len(listedMarkets) {
		t.Errorf("got %d tickers, want one per listed market (%d)",
			len(tickers.Tickers), len(listedMarkets))
	}
	// Only BTC-USD traded above; the rest must still be reported, as untraded.
	for _, ticker := range tickers.Tickers {
		if ticker.Market == "BTC-USD" && !ticker.HasTraded {
			t.Error("BTC-USD reports no trades after one executed")
		}
		if ticker.Market != "BTC-USD" && ticker.HasTraded {
			t.Errorf("%s reports trades it never had", ticker.Market)
		}
	}

	var book struct {
		Market string `json:"market"`
		Bids   []struct {
			Price, Quantity int64
			Orders          int
		} `json:"bids"`
		Asks []struct {
			Price, Quantity int64
			Orders          int
		} `json:"asks"`
	}
	rec = a.do(http.MethodGet, "/orderbook/BTC-USD", "", nil)
	a.expect(rec, http.StatusOK, "GET /orderbook/{symbol}")
	a.decode(rec, &book)
	if book.Market != "BTC-USD" {
		t.Errorf("book market = %q", book.Market)
	}
	if len(book.Asks) != 1 || book.Asks[0].Quantity != 50_000_000 {
		t.Errorf("asks = %+v, want the unsold half still resting", book.Asks)
	}

	// The caller's own executions need a token; the tape does not.
	var fills models.Fills
	rec = a.do(http.MethodGet, "/trades", buyer, nil)
	a.expect(rec, http.StatusOK, "GET /trades")
	a.decode(rec, &fills)
	if len(fills.Trades) != 1 || fills.Trades[0].Side != "buy" || !fills.Trades[0].Taker {
		t.Errorf("buyer fills = %+v, want one taker buy", fills.Trades)
	}
	a.expect(a.do(http.MethodGet, "/trades", "", nil), http.StatusUnauthorized, "unauthenticated /trades")
}

func TestUnknownMarketsAre404(t *testing.T) {
	a := newAPI(t)

	for _, path := range []string{
		"/markets/NOPE-USD/ticker",
		"/markets/NOPE-USD/trades",
		"/orderbook/NOPE-USD",
	} {
		a.expect(a.do(http.MethodGet, path, "", nil), http.StatusNotFound, path)
	}
}

func TestReferenceDataIsPublic(t *testing.T) {
	a := newAPI(t)

	rec := a.do(http.MethodGet, "/currencies", "", nil)
	a.expect(rec, http.StatusOK, "GET /currencies")
	var currencies []struct {
		Code     string `json:"code"`
		Exponent int    `json:"exponent"`
	}
	a.decode(rec, &currencies)
	if len(currencies) < 2 {
		t.Fatalf("currencies = %+v", currencies)
	}
	for _, c := range currencies {
		if c.Code == "" {
			t.Error("a currency has no code")
		}
	}

	rec = a.do(http.MethodGet, "/markets", "", nil)
	a.expect(rec, http.StatusOK, "GET /markets")
	var markets []struct {
		Symbol, Base, Quote string
	}
	a.decode(rec, &markets)

	listed, _ := market.Default()
	_ = listed
	if len(markets) == 0 {
		t.Fatal("no markets listed")
	}
	// Listing order is part of the contract: the frontend defaults to the first.
	if markets[0].Symbol != "BTC-USD" {
		t.Errorf("first market = %s, want BTC-USD", markets[0].Symbol)
	}
	for _, m := range markets {
		if m.Symbol == "" || m.Base == "" || m.Quote == "" {
			t.Errorf("market %+v is missing a field", m)
		}
	}
}
