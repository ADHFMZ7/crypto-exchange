package api

import (
	"net/http"

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
		"OPTIONS /trades/",
		http.HandlerFunc(emptyHandler),
	)
	mux.Handle(
		"POST /trades",
		Authenticate(http.HandlerFunc(router.CreateTrade)),
	)
}

func (router *TradeRouter) CreateTrade(w http.ResponseWriter, r *http.Request) {
	// POST /trades/new - Create a new trade
	// Responses:
	// 200 OK - Trade request submitted succesfully
	// 401 Unauthorized - user not authenticated
	// 404 Not Found - wallet not found

	// ctx := r.Context()
	// userID, ok := auth.UserIDFromContext(ctx)
	// if !ok {
	// 	http.Error(w, "unauthorized", http.StatusUnauthorized)
	// 	return
	// }

	// verify if trade is legal

}
