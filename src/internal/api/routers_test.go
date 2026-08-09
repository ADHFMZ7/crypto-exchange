package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ADHFMZ7/crypto-exchange/internal/auth"
	"github.com/ADHFMZ7/crypto-exchange/internal/market"
	"github.com/ADHFMZ7/crypto-exchange/internal/services"
)

/*
Handler tests that need no database.

Every case below exercises a guard clause — a request that is rejected before
the handler reaches through to a service. That boundary is exactly why these
routers can be tested at all with a nil *services.Services: a test that got past
the guard would panic instead, which makes "did validation actually run?" an
unusually honest assertion here.

Anything past the guards needs a live Postgres and is not covered.
*/

// authed returns a request carrying the user id Authenticate would have put
// there, so a handler can be reached without minting a token.
func authed(r *http.Request, userID int64) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), auth.CtxUserKey{}, userID))
}

func postJSON(path, body string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	return req
}

func TestCreateOrderRejectsBadRequests(t *testing.T) {
	router := &OrderRouter{}

	cases := []struct {
		name       string
		body       string
		authorised bool
		wantStatus int
	}{
		{
			name:       "unauthenticated",
			body:       `{"market":"BTC-USD","side":"buy","quantity":10000000,"price":4500000}`,
			authorised: false,
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "malformed json",
			body:       `{"market":`,
			authorised: true,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "empty body",
			body:       ``,
			authorised: true,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "missing market",
			body:       `{"side":"buy","quantity":10000000,"price":4500000}`,
			authorised: true,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "blank market",
			body:       `{"market":"","side":"buy","quantity":10000000,"price":4500000}`,
			authorised: true,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "quantity is a string",
			body:       `{"market":"BTC-USD","side":"buy","quantity":"10000000","price":4500000}`,
			authorised: true,
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := postJSON("/orders", tc.body)
			if tc.authorised {
				req = authed(req, 42)
			}
			rec := httptest.NewRecorder()

			router.CreateOrder(rec, req)

			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d (body %q)", rec.Code, tc.wantStatus, rec.Body.String())
			}
		})
	}
}

func TestHandlersRequireAuthenticatedContext(t *testing.T) {
	// Authenticate normally guarantees the context value, but each of these
	// checks it again rather than trusting the wiring. A regression here is a
	// route serving another user's data.
	cases := map[string]http.HandlerFunc{
		"users/me":         (&UserRouter{}).UserGetSelf,
		"wallets/me get":   (&WalletRouter{}).GetWalletSelf,
		"wallets/me patch": (&WalletRouter{}).DepositToWallet,
		"orders create":    (&OrderRouter{}).CreateOrder,
		"orders list":      (&OrderRouter{}).GetOrders,
	}

	for name, handler := range cases {
		t.Run(name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			handler(rec, httptest.NewRequest(http.MethodGet, "/", strings.NewReader("{}")))

			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want 401", rec.Code)
			}
		})
	}
}

func TestDepositRejectsMalformedBody(t *testing.T) {
	router := &WalletRouter{}

	for name, body := range map[string]string{
		"malformed":             `{"Amount":`,
		"empty":                 ``,
		"not json":              `deposit 100`,
		"wrong type for amount": `{"Amount":"one hundred"}`,
	} {
		t.Run(name, func(t *testing.T) {
			req := authed(postJSON("/wallets/me", body), 42)
			rec := httptest.NewRecorder()

			router.DepositToWallet(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400", rec.Code)
			}
		})
	}
}

func TestUserGetHandlerRejectsNonNumericID(t *testing.T) {
	router := &UserRouter{}

	for _, id := range []string{"", "abc", "1.5", "9999999999999999999999", "1; DROP TABLE users"} {
		t.Run("id="+id, func(t *testing.T) {
			// The URL is a fixed placeholder: ServeMux would have extracted the
			// path value, and some of these ids are not legal in a request line.
			req := httptest.NewRequest(http.MethodGet, "/users/placeholder", nil)
			req.SetPathValue("id", id)
			rec := httptest.NewRecorder()

			router.UserGetHandler(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400", rec.Code)
			}
		})
	}
}

func TestLoginAndRegisterRejectMalformedJSON(t *testing.T) {
	authRouter := &AuthRouter{}
	userRouter := &UserRouter{}

	for name, body := range map[string]string{
		"truncated": `{"email":`,
		"empty":     ``,
		"array":     `["email","password"]`,
	} {
		t.Run("login/"+name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			authRouter.LoginHandler(rec, postJSON("/auth/login", body))
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400", rec.Code)
			}
		})

		t.Run("register/"+name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			userRouter.UserRegister(rec, postJSON("/users", body))
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400", rec.Code)
			}
		})
	}
}

// Logout is stateless by design: the token stays valid until it expires, and
// the client is what forgets it. Pinning this stops it quietly becoming a
// no-op that returns an error.
func TestLogoutSucceedsAndReturnsNothing(t *testing.T) {
	rec := httptest.NewRecorder()
	(&AuthRouter{}).LogoutHandler(rec, httptest.NewRequest(http.MethodPost, "/auth/logout", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if rec.Body.Len() != 0 {
		t.Fatalf("body = %q, want empty", rec.Body.String())
	}
}

// minimalServices builds the smallest *services.Services that NewRouter can be
// wired with: NewReferenceRouter reads the registry through it at construction,
// so a nil aggregate panics. The struct literal deliberately bypasses
// NewOrderService, which needs a database and starts a worker goroutine per
// market that would outlive the test.
func minimalServices(t *testing.T) *services.Services {
	t.Helper()

	registry, err := market.NewMarketRegistry(market.Default())
	if err != nil {
		t.Fatal(err)
	}

	return &services.Services{
		Orders: &services.OrderService{Registry: registry},
	}
}

// The route table itself: method and path, without a database behind it.
// Protected routes answer 401, so reaching the guard proves the route resolved.
func TestRouterRouteTable(t *testing.T) {
	mux := NewRouter(minimalServices(t))

	cases := []struct {
		method string
		path   string
		want   int
	}{
		{http.MethodGet, "/users/me", http.StatusUnauthorized},
		{http.MethodGet, "/wallets/me", http.StatusUnauthorized},
		{http.MethodPatch, "/wallets/me", http.StatusUnauthorized},
		{http.MethodPost, "/orders", http.StatusUnauthorized},
		{http.MethodGet, "/orders", http.StatusUnauthorized},

		// Preflight routes answer without authentication.
		{http.MethodOptions, "/orders/", http.StatusOK},
		{http.MethodOptions, "/wallets/", http.StatusOK},
		{http.MethodOptions, "/users/", http.StatusOK},
		{http.MethodOptions, "/auth/", http.StatusOK},

		// Public reference data: no auth, no database.
		{http.MethodGet, "/currencies", http.StatusOK},
		{http.MethodGet, "/markets", http.StatusOK},

		// Public, and rejected here only because the body is empty.
		{http.MethodPost, "/auth/login", http.StatusBadRequest},
		{http.MethodPost, "/users", http.StatusBadRequest},

		// Not routed. /trades is retired — the order router replaces it.
		{http.MethodGet, "/does-not-exist", http.StatusNotFound},
		{http.MethodPost, "/trades", http.StatusNotFound},
		{http.MethodDelete, "/users/me", http.StatusMethodNotAllowed},
	}

	for _, tc := range cases {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, httptest.NewRequest(tc.method, tc.path, strings.NewReader("")))

			if rec.Code != tc.want {
				t.Fatalf("status = %d, want %d", rec.Code, tc.want)
			}
		})
	}
}
