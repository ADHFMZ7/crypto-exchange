package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

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
		"OPTIONS /trades/",
		http.HandlerFunc(emptyHandler),
	)
	mux.Handle(
		"POST /trades",
		Authenticate(http.HandlerFunc(router.CreateTrade)),
	)
}

func (router *TradeRouter) CreateTrade(w http.ResponseWriter, r *http.Request) {
	// POST /trades - Create a new trade
	// Responses:
	// 202 Accepted - Trade request submitted successfully
	// 400 Bad Request - invalid request payload
	// 401 Unauthorized - user not authenticated
	// 404 Not Found - unknown market
	// 503 Service Unavailable - trade queue is full

	ctx := r.Context()
	_, ok := auth.UserIDFromContext(ctx)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var payload struct {
		OrderID int64  `json:"order_id"`
		Market  string `json:"market"`
		Type    string `json:"type"`
		Shares  int64  `json:"shares"`
		Price   int64  `json:"price"`
	}

	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if payload.Market == "" {
		http.Error(w, "market is required", http.StatusBadRequest)
		return
	}

	requestType, valid := parseTradeType(payload.Type)
	if !valid {
		http.Error(w, "type must be one of: limit_buy, limit_sell, cancel", http.StatusBadRequest)
		return
	}

	orderID := payload.OrderID

	if requestType != services.Cancel {
		if payload.Shares <= 0 || payload.Price <= 0 {
			http.Error(w, "shares and price must be positive for limit orders", http.StatusBadRequest)
			return
		}
		orderID = router.Services.Trades.NextOrderID()
	} else if orderID <= 0 {
		http.Error(w, "order_id is required for cancel", http.StatusBadRequest)
		return
	}

	queue, ok := router.Services.Trades.RQueues[payload.Market]
	if !ok {
		http.Error(w, "market not found", http.StatusNotFound)
		return
	}

	req := services.Request{
		Type:    requestType,
		OrderID: orderID,
		Price:   payload.Price,
		Shares:  payload.Shares,
	}

	select {
	case queue <- req:
	default:
		http.Error(w, "trade queue is full", http.StatusServiceUnavailable)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status":     "accepted",
		"order_id":   orderID,
		"market":     payload.Market,
		"type":       payload.Type,
		"receivedAt": time.Now().UTC().Format(time.RFC3339),
	})

}

func parseTradeType(raw string) (services.RequestType, bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "limit_buy", "buy":
		return services.LimitBuy, true
	case "limit_sell", "sell":
		return services.LimitSell, true
	case "cancel":
		return services.Cancel, true
	default:
		return services.LimitBuy, false
	}
}
