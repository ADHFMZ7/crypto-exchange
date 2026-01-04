package api

import (
	"encoding/json"
	"net/http"

	"github.com/ADHFMZ7/crypto-exchange/internal/auth"
	"github.com/ADHFMZ7/crypto-exchange/internal/services"
)

type WalletRouter struct {
	Services *services.Services
}

func NewWalletRouter(service *services.Services) *WalletRouter {
	return &WalletRouter{Services: service}
}

func (router *WalletRouter) Register(mux *http.ServeMux) {

	mux.Handle(
		"OPTIONS /wallets/",
		http.HandlerFunc(emptyHandler),
	)
	mux.Handle(
		"GET /wallets/me",
		auth.AuthMiddleware(http.HandlerFunc(router.GetWalletSelf)),
	)
}

// Handlers below here
func (router *WalletRouter) GetWalletByUserID(w http.ResponseWriter, r *http.Request) {
	//
}

func (router *WalletRouter) GetWalletSelf(w http.ResponseWriter, r *http.Request) {
	// GET /wallets/me - get wallet of authenticated user
	// Responses:
	// 200 OK - wallet retrieved successfully
	// 401 Unauthorized - user not authenticated
	// 404 Not Found - wallet not found

	ctx := r.Context()
	// userID, ok := ctx.Value(auth.CtxUserKey).(int64)
	userID := int64(r.Context().Value(auth.CtxUserKey).(int))

	// if !ok {
	// 	http.Error(w, "unauthorized", http.StatusUnauthorized)
	// 	return
	// }

	wallet, err := router.Services.Wallets.GetWalletByUserID(ctx, userID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	err = json.NewEncoder(w).Encode(wallet)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}
