package auth

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

// Simple JWT implementation using HMAC-SHA256. Intentionally minimal to
// avoid external dependencies while providing secure defaults.

type ctxKey string

const (
	CtxUserKey ctxKey = "user_id"
	// default token TTL for generated tokens
	defaultTTL = 15 * time.Minute
)

// Claims is the minimal JWT claims payload we use.
type Claims struct {
	Sub string `json:"sub"` // subject (user id)
	Iat int64  `json:"iat"`
	Exp int64  `json:"exp"`
}

// jwtSecret is read from the environment variable JWT_SECRET. If not set,
// a non-production default is used. Make sure to set a strong secret in
// production.
var jwtSecret = func() []byte {
	s := os.Getenv("JWT_SECRET")
	if s == "" {
		s = "dev-secret-change-me"
	}
	return []byte(s)
}()

// GenerateJWT returns a signed JWT for the given user id. ttl is optional
// (use 0 for the default TTL).
func GenerateJWT(userID string, ttl time.Duration) (string, error) {
	if ttl <= 0 {
		ttl = defaultTTL
	}

	now := time.Now().UTC()
	claims := Claims{
		Sub: userID,
		Iat: now.Unix(),
		Exp: now.Add(ttl).Unix(),
	}

	header := map[string]string{"alg": "HS256", "typ": "JWT"}

	hJSON, err := json.Marshal(header)
	if err != nil {
		return "", err
	}
	cJSON, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}

	enc := base64.RawURLEncoding
	headerB := enc.EncodeToString(hJSON)
	claimsB := enc.EncodeToString(cJSON)

	signingInput := headerB + "." + claimsB
	sig := sign([]byte(signingInput))
	token := signingInput + "." + enc.EncodeToString(sig)
	return token, nil
}

func ValidateJWT(authHeader string) (int, bool) {
	// ValidateJWT verifies the token signature and expiration. It accepts either
	// the raw token or a header value starting with "Bearer ". Returns the subject
	// (user id) and true on success.
	if strings.HasPrefix(strings.ToLower(authHeader), "bearer ") {
		authHeader = strings.TrimSpace(authHeader[7:])
	}
	parts := strings.Split(authHeader, ".")
	if len(parts) != 3 {
		return -1, false
	}

	enc := base64.RawURLEncoding
	headerB, err := enc.DecodeString(parts[0])
	if err != nil || len(headerB) == 0 {
		return -1, false
	}
	claimsB, err := enc.DecodeString(parts[1])
	if err != nil || len(claimsB) == 0 {
		return -1, false
	}
	sig, err := enc.DecodeString(parts[2])
	if err != nil {
		return -1, false
	}

	// verify signature
	signingInput := parts[0] + "." + parts[1]
	expected := sign([]byte(signingInput))
	if !hmac.Equal(sig, expected) {
		return -1, false
	}

	var claims Claims
	if err := json.Unmarshal(claimsB, &claims); err != nil {
		return -1, false
	}

	if time.Now().UTC().Unix() > claims.Exp {
		return -1, false
	}

	id, err := strconv.Atoi(claims.Sub)
	if err != nil {
		return -1, false
	}
	return id, true
}

func sign(data []byte) []byte {
	// sign produces HMAC-SHA256 signature for the given data using jwtSecret.
	mac := hmac.New(sha256.New, jwtSecret)
	mac.Write(data)
	return mac.Sum(nil)
}

func AuthMiddleware(next http.Handler) http.Handler {
	// AuthMiddleware checks the Authorization header for a valid JWT and, on
	// success, injects the user id into the request context under the key
	// `user_id`.
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth == "" {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		uid, ok := ValidateJWT(auth)
		if !ok {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		fmt.Println("Authenticated user ID:", uid)
		ctx := context.WithValue(r.Context(), CtxUserKey, uid)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func UserIDFromContext(ctx context.Context) (string, bool) {
	// UserIDFromContext retrieves the user id (subject) from a request context.

	v := ctx.Value(CtxUserKey)
	if v == nil {
		return "", false
	}
	s, ok := v.(string)
	return s, ok
}
