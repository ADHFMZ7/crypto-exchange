package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ADHFMZ7/crypto-exchange/internal/auth"
)

// recorderHandler reports whether the wrapped handler was reached, which is the
// property that actually matters for middleware: rejecting a request means the
// handler behind it never runs.
type recorderHandler struct {
	called bool
	userID int64
	hasID  bool
}

func (h *recorderHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.called = true
	h.userID, h.hasID = auth.UserIDFromContext(r.Context())
	w.WriteHeader(http.StatusOK)
}

func TestAuthenticatePassesValidTokenThrough(t *testing.T) {
	token, err := auth.GenerateJWT("42", time.Minute)
	if err != nil {
		t.Fatal(err)
	}

	for name, header := range map[string]string{
		"bare token":    token,
		"bearer scheme": "Bearer " + token,
	} {
		t.Run(name, func(t *testing.T) {
			next := &recorderHandler{}
			req := httptest.NewRequest(http.MethodGet, "/users/me", nil)
			req.Header.Set("Authorization", header)
			rec := httptest.NewRecorder()

			Authenticate(next).ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200", rec.Code)
			}
			if !next.called {
				t.Fatal("handler was not reached")
			}
			// The whole reason this middleware exists: downstream handlers read
			// the caller's identity from the context, never from the request.
			if !next.hasID || next.userID != 42 {
				t.Fatalf("context user = (%d, %v), want (42, true)", next.userID, next.hasID)
			}
		})
	}
}

func TestAuthenticateRejects(t *testing.T) {
	valid, err := auth.GenerateJWT("42", time.Minute)
	if err != nil {
		t.Fatal(err)
	}

	// Mutate the FIRST character of the signature, not the last. A 32-byte
	// signature is 43 base64 characters — 258 bits of alphabet for 256 bits of
	// data — so the final character carries 2 unused bits, and swapping it
	// between 'A' and 'B' yields a different string that decodes to identical
	// bytes. The first character always contributes real bits.
	parts := strings.Split(valid, ".")
	sig := []byte(parts[2])
	if sig[0] == 'A' {
		sig[0] = 'B'
	} else {
		sig[0] = 'A'
	}
	tampered := parts[0] + "." + parts[1] + "." + string(sig)

	for name, header := range map[string]string{
		"missing":          "",
		"garbage":          "not-a-token",
		"bearer only":      "Bearer ",
		"tampered":         tampered,
		"wrong scheme":     "Basic " + valid,
		"empty bearer arg": "Bearer",
	} {
		t.Run(name, func(t *testing.T) {
			next := &recorderHandler{}
			req := httptest.NewRequest(http.MethodGet, "/users/me", nil)
			if header != "" {
				req.Header.Set("Authorization", header)
			}
			rec := httptest.NewRecorder()

			Authenticate(next).ServeHTTP(rec, req)

			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want 401", rec.Code)
			}
			if next.called {
				t.Fatal("handler ran despite failed authentication")
			}
		})
	}
}

func TestWithCORSSetsHeadersAndCallsThrough(t *testing.T) {
	next := &recorderHandler{}
	req := httptest.NewRequest(http.MethodGet, "/wallets/me", nil)
	rec := httptest.NewRecorder()

	WithCORS(next).ServeHTTP(rec, req)

	if !next.called {
		t.Fatal("non-preflight request did not reach the handler")
	}

	want := map[string]string{
		"Access-Control-Allow-Origin":  "http://localhost:5173",
		"Access-Control-Allow-Headers": "Content-Type, Authorization",
	}
	for header, expected := range want {
		if got := rec.Header().Get(header); got != expected {
			t.Fatalf("%s = %q, want %q", header, got, expected)
		}
	}
}

// The browser refuses the request when the method is absent from the preflight
// response, and curl does not preflight — so a missing method here is invisible
// from the terminal and breaks only the real app. PATCH is the deposit path.
func TestWithCORSAllowsEveryMethodTheAppUses(t *testing.T) {
	rec := httptest.NewRecorder()
	WithCORS(&recorderHandler{}).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	allowed := rec.Header().Get("Access-Control-Allow-Methods")
	for _, method := range []string{"GET", "POST", "PATCH", "OPTIONS", "DELETE", "PUT"} {
		if !strings.Contains(allowed, method) {
			t.Fatalf("Access-Control-Allow-Methods = %q, missing %s", allowed, method)
		}
	}
}

func TestWithCORSShortCircuitsPreflight(t *testing.T) {
	next := &recorderHandler{}
	req := httptest.NewRequest(http.MethodOptions, "/orders", nil)
	rec := httptest.NewRecorder()

	WithCORS(next).ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}
	if next.called {
		t.Fatal("preflight reached the handler; it should be answered by the middleware")
	}
	if rec.Header().Get("Access-Control-Allow-Origin") == "" {
		t.Fatal("preflight response carried no CORS headers, which is the one thing it is for")
	}
}

func TestEmptyHandlerIsANoOp(t *testing.T) {
	rec := httptest.NewRecorder()
	emptyHandler(rec, httptest.NewRequest(http.MethodOptions, "/orders/", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if rec.Body.Len() != 0 {
		t.Fatalf("body = %q, want empty", rec.Body.String())
	}
}
