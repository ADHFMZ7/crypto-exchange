package auth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// craftToken builds a token signed with the package's real secret, so tests can
// produce claims GenerateJWT will not emit — an expired token, a non-numeric
// subject — without reaching for a second implementation of the format.
func craftToken(t *testing.T, claims Claims) string {
	t.Helper()

	enc := base64.RawURLEncoding

	hJSON, err := json.Marshal(map[string]string{"alg": "HS256", "typ": "JWT"})
	if err != nil {
		t.Fatal(err)
	}
	cJSON, err := json.Marshal(claims)
	if err != nil {
		t.Fatal(err)
	}

	signingInput := enc.EncodeToString(hJSON) + "." + enc.EncodeToString(cJSON)
	return signingInput + "." + enc.EncodeToString(sign([]byte(signingInput)))
}

func TestGenerateAndValidateRoundTrip(t *testing.T) {
	token, err := GenerateJWT("42", time.Minute)
	if err != nil {
		t.Fatal(err)
	}

	id, ok := ValidateJWT(token)
	if !ok {
		t.Fatal("freshly generated token was rejected")
	}
	if id != 42 {
		t.Fatalf("subject = %d, want 42", id)
	}
}

// The header arrives straight from the Authorization header, so the scheme
// prefix has to come off — in whatever case the client sent it.
func TestValidateAcceptsBearerPrefixInAnyCase(t *testing.T) {
	token, err := GenerateJWT("7", time.Minute)
	if err != nil {
		t.Fatal(err)
	}

	for _, header := range []string{
		token,
		"Bearer " + token,
		"bearer " + token,
		"BEARER " + token,
		"BeArEr   " + token, // TrimSpace handles the padding
	} {
		id, ok := ValidateJWT(header)
		if !ok || id != 7 {
			t.Fatalf("ValidateJWT(%.20q…) = (%d, %v), want (7, true)", header, id, ok)
		}
	}
}

// A zero or negative TTL means "use the default", so this cannot express an
// already-expired token — which is why craftToken exists.
func TestNonPositiveTTLFallsBackToDefault(t *testing.T) {
	for _, ttl := range []time.Duration{0, -time.Hour} {
		token, err := GenerateJWT("1", ttl)
		if err != nil {
			t.Fatal(err)
		}

		var claims Claims
		decodeClaims(t, token, &claims)

		got := time.Unix(claims.Exp, 0).Sub(time.Unix(claims.Iat, 0))
		if got != defaultTTL {
			t.Fatalf("ttl %v produced a %v token, want the %v default", ttl, got, defaultTTL)
		}
	}
}

func TestExpiredTokenIsRejected(t *testing.T) {
	now := time.Now().UTC()

	expired := craftToken(t, Claims{
		Sub: "42",
		Iat: now.Add(-2 * time.Hour).Unix(),
		Exp: now.Add(-time.Second).Unix(),
	})

	if id, ok := ValidateJWT(expired); ok {
		t.Fatalf("expired token accepted as user %d", id)
	}

	// Signature is intact — expiry is the only reason it failed.
	valid := craftToken(t, Claims{Sub: "42", Iat: now.Unix(), Exp: now.Add(time.Hour).Unix()})
	if _, ok := ValidateJWT(valid); !ok {
		t.Fatal("control token with the same construction was rejected")
	}
}

// The whole point of the signature: claims cannot be edited in transit.
func TestTamperedClaimsAreRejected(t *testing.T) {
	token, err := GenerateJWT("42", time.Minute)
	if err != nil {
		t.Fatal(err)
	}

	parts := strings.Split(token, ".")
	forged, err := json.Marshal(Claims{
		Sub: "1", // escalate to another user
		Iat: time.Now().UTC().Unix(),
		Exp: time.Now().UTC().Add(time.Hour).Unix(),
	})
	if err != nil {
		t.Fatal(err)
	}

	swapped := parts[0] + "." + base64.RawURLEncoding.EncodeToString(forged) + "." + parts[2]

	if id, ok := ValidateJWT(swapped); ok {
		t.Fatalf("token with swapped claims accepted as user %d", id)
	}
}

func TestTokenSignedWithAnotherSecretIsRejected(t *testing.T) {
	// Built by hand rather than through sign(), so it carries a signature this
	// process could never have produced.
	enc := base64.RawURLEncoding
	header := enc.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
	claims := enc.EncodeToString([]byte(`{"sub":"42","iat":0,"exp":99999999999}`))
	foreign := header + "." + claims + "." + enc.EncodeToString([]byte("not-a-real-signature"))

	if id, ok := ValidateJWT(foreign); ok {
		t.Fatalf("foreign signature accepted as user %d", id)
	}
}

func TestMalformedTokensAreRejected(t *testing.T) {
	valid, err := GenerateJWT("42", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(valid, ".")

	cases := map[string]string{
		"empty":                "",
		"no dots":              "notatoken",
		"two parts":            parts[0] + "." + parts[1],
		"four parts":           valid + ".extra",
		"empty header":         "." + parts[1] + "." + parts[2],
		"empty claims":         parts[0] + ".." + parts[2],
		"header not base64":    "!!!." + parts[1] + "." + parts[2],
		"claims not base64":    parts[0] + ".!!!." + parts[2],
		"signature not base64": parts[0] + "." + parts[1] + ".!!!",
		"claims not json":      parts[0] + "." + base64.RawURLEncoding.EncodeToString([]byte("nope")) + "." + parts[2],
		"bearer with no token": "Bearer ",
	}

	for name, token := range cases {
		t.Run(name, func(t *testing.T) {
			if id, ok := ValidateJWT(token); ok {
				t.Fatalf("accepted as user %d", id)
			}
		})
	}
}

// The subject is a string in the token but an int64 to the rest of the app, so
// anything that is not a number has to fail closed rather than land as 0.
func TestNonNumericSubjectIsRejected(t *testing.T) {
	token := craftToken(t, Claims{
		Sub: "admin",
		Iat: time.Now().UTC().Unix(),
		Exp: time.Now().UTC().Add(time.Hour).Unix(),
	})

	if id, ok := ValidateJWT(token); ok {
		t.Fatalf("non-numeric subject accepted as user %d", id)
	}
}

func TestUserIDFromContext(t *testing.T) {
	t.Run("present", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), CtxUserKey{}, int64(42))
		id, ok := UserIDFromContext(ctx)
		if !ok || id != 42 {
			t.Fatalf("got (%d, %v), want (42, true)", id, ok)
		}
	})

	t.Run("absent", func(t *testing.T) {
		if id, ok := UserIDFromContext(context.Background()); ok {
			t.Fatalf("empty context yielded user %d", id)
		}
	})

	t.Run("wrong type", func(t *testing.T) {
		// Authenticate stores an int64; anything else must not be coerced.
		ctx := context.WithValue(context.Background(), CtxUserKey{}, "42")
		if id, ok := UserIDFromContext(ctx); ok {
			t.Fatalf("string value yielded user %d", id)
		}
	})
}

func decodeClaims(t *testing.T, token string, into *Claims) {
	t.Helper()

	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatalf("token has %d parts, want 3", len(parts))
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, into); err != nil {
		t.Fatal(err)
	}
}
