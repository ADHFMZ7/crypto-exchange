package api

import (
	"net/http"

	"github.com/ADHFMZ7/crypto-exchange/internal/services"
	"github.com/ADHFMZ7/crypto-exchange/internal/stream"
)

// NewRouter mounts every route.
//
// hub may be nil, which simply leaves the live feed unmounted — the REST
// endpoints are the authority either way, so a process without a stream is a
// working exchange with a chattier frontend.
func NewRouter(services *services.Services, hub *stream.Hub) *http.ServeMux {

	// TODO: Refactor to make middleware less clunky

	mux := http.NewServeMux()

	NewUserRouter(services).Register(mux)
	NewAuthRouter(services).Register(mux)
	NewWalletRouter(services).Register(mux)
	NewTradeRouter(services).Register(mux)
	NewReferenceRouter(services).Register(mux)
	NewOrderRouter(services).Register(mux)
	NewMarketRouter(services).Register(mux)

	if hub != nil {
		NewStreamRouter(services, hub).Register(mux)
	}

	return mux
}
