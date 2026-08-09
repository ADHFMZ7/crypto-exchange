package api

import (
	"github.com/ADHFMZ7/crypto-exchange/internal/services"
)

type TradeRouter struct {
	Services *services.Services
}

func NewTradeRouter(service *services.Services) *TradeRouter {
	return &TradeRouter{Services: service}
}

// func (router *TradeRouter) Register(mux *http.ServeMux) {

// 	mux.Handle(
// 		"OPTIONS /trades/",
// 		http.HandlerFunc(emptyHandler),
// 	)
// 	mux.Handle(
// 		"POST /trades",
// 		Authenticate(http.HandlerFunc(router.CreateTrade)),
// 	)
// mux.Handle(
// 	"GET /trades",
// 	Authenticate(i),
// )
// }

// TODO: Move this later

// func (router *TradeRouter) CreateTrade(w http.ResponseWriter, r *http.Request) {
// 	// POST /trades - Create a new trade
// 	// Responses:
// 	// 202 Accepted - Trade request submitted successfully
// 	// 400 Bad Request - invalid request payload
// 	// 401 Unauthorized - user not authenticated
// 	// 404 Not Found - unknown market
// 	// 503 Service Unavailable - trade queue is full

// 	ctx := r.Context()
// 	userid, ok := auth.UserIDFromContext(ctx)
// 	if !ok {
// 		http.Error(w, "unauthorized", http.StatusUnauthorized)
// 		return
// 	}

// 	var payload struct {
// 		OrderID int64  `json:"order_id"`
// 		Market  string `json:"market"`
// 		Type    string `json:"type"`
// 		Shares  int64  `json:"shares"`
// 		Price   int64  `json:"price"`
// 	}

// 	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
// 		http.Error(w, err.Error(), http.StatusBadRequest)
// 		return
// 	}

// 	if payload.Market == "" {
// 		http.Error(w, "market is required", http.StatusBadRequest)
// 		return
// 	}

// requestType, valid := parseOrderType(payload.Type)
// if !valid {
// 	http.Error(w, "type must be one of: limit_buy, limit_sell, cancel", http.StatusBadRequest)
// 	return
// }

// orderID := payload.OrderID
// market, _ := router.Services.Trades.MarketRegistry.BySymbol(payload.Market)

// switch requestType {
// case services.LimitSell:
// 	router.Services.Trades.LimitSell(ctx, userid, market, payload.Shares, payload.Price)
// case services.LimitBuy:
// 	router.Services.Trades.LimitBuy(ctx, userid, market, payload.Price, payload.Shares)
// case services.Cancel:
// router.Services.Trades.Cancel()
// }

// if requestType != services.Cancel {
// 	if payload.Shares <= 0 || payload.Price <= 0 {
// 		http.Error(w, "shares and price must be positive for limit orders", http.StatusBadRequest)
// 		return
// 	}
// 	orderID = router.Services.Trades.NextOrderID()
// } else if orderID <= 0 {
// 	http.Error(w, "order_id is required for cancel", http.StatusBadRequest)
// 	return
// }

// 	w.Header().Set("Content-Type", "application/json")
// 	w.WriteHeader(http.StatusAccepted)
// 	_ = json.NewEncoder(w).Encode(map[string]any{
// 		"status":     "accepted",
// 		"order_id":   orderID,
// 		"market":     payload.Market,
// 		"type":       payload.Type,
// 		"receivedAt": time.Now().UTC().Format(time.RFC3339),
// 	})

// }
