package api

import (
	"net/http"
	"strconv"

	"github.com/ADHFMZ7/crypto-exchange/internal/auth"
	"github.com/ADHFMZ7/crypto-exchange/internal/services"
)

type TradeRouter struct {
	Services *services.Services
}

func NewTradeRouter(service *services.Services) *TradeRouter {
	return &TradeRouter{Services: service}
}

func (router *TradeRouter) Register(mux *http.ServeMux) {
	mux.Handle(
		"OPTIONS /trades",
		http.HandlerFunc(emptyHandler),
	)
	mux.Handle(
		"GET /trades",
		Authenticate(http.HandlerFunc(router.GetTrades)),
	)
}

// GetTrades serves the caller's own executions.
//
// This is the detail behind filled_quantity on GET /orders: that says how much
// of an order has filled, this says which trades did it, at what price and
// when. An order that walked several price levels has one row here per level.
func (router *TradeRouter) GetTrades(w http.ResponseWriter, r *http.Request) {

	ctx := r.Context()

	userID, ok := auth.UserIDFromContext(ctx)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	fills, err := router.Services.Trades.FillsForUser(ctx, userID, limitParam(r))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load trades")
		return
	}

	writeJSON(w, http.StatusOK, fills)
}

// limitParam reads ?limit=, falling back to the service default. A value that
// is not a number is treated as absent rather than rejected: it only affects
// page size, and failing the whole read over it helps nobody.
func limitParam(r *http.Request) int {
	limit, err := strconv.Atoi(r.URL.Query().Get("limit"))
	if err != nil {
		return 0
	}
	return limit
}
