package api

import (
	"context"
	"fmt"
	"net/http"

	"github.com/ADHFMZ7/crypto-exchange/internal/auth"
)

// TODO: Find a better way to chain middlewares
// TODO: Add logging middleware

func emptyHandler(w http.ResponseWriter, r *http.Request) {}

func Authenticate(next http.Handler) http.Handler {
	// AuthMiddleware checks the Authorization header for a valid JWT and, on
	// success, injects the user id into the request context under the key
	// `user_id`.
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth_header := r.Header.Get("Authorization")
		if auth_header == "" {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		uid, ok := auth.ValidateJWT(auth_header)
		if !ok {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		fmt.Println("Authenticated user ID:", uid)
		ctx := context.WithValue(r.Context(), auth.CtxUserKey{}, uid)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func WithCORS(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// React dev server
		w.Header().Set("Access-Control-Allow-Origin", "http://localhost:5173")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		// Preflight request
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		h.ServeHTTP(w, r)
	})
}

// util functions for middleware chaining
