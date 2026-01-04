package api

import (
	"net/http"

	"github.com/ADHFMZ7/crypto-exchange/internal/services"
)

func NewRouter(services *services.Services) *http.ServeMux {

	// TODO: Refactor to make middleware less clunky

	mux := http.NewServeMux()

	NewUserRouter(services).Register(mux)
	NewAuthRouter(services).Register(mux)
	NewWalletRouter(services).Register(mux)

	return mux
}
