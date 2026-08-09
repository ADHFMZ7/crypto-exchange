package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/ADHFMZ7/crypto-exchange/internal/auth"
	"github.com/ADHFMZ7/crypto-exchange/internal/market"
	"github.com/ADHFMZ7/crypto-exchange/internal/services"
)

type OrderRouter struct {
	Services *services.Services
	Registry *market.Registry
}

func NewOrderRouter(service *services.Services) *OrderRouter {
	return &OrderRouter{Services: service, Registry: service.Orders.Registry}
}

func (router *OrderRouter) Register(mux *http.ServeMux) {
	mux.Handle(
		"OPTIONS /orders/",
		http.HandlerFunc(emptyHandler),
	)
	mux.Handle(
		"POST /orders",
		Authenticate(http.HandlerFunc(router.CreateOrder)),
	)
	mux.Handle(
		"GET /orders",
		Authenticate(http.HandlerFunc(router.GetOrders)),
	)
}

func (router *OrderRouter) GetOrders(w http.ResponseWriter, r *http.Request) {

	ctx := r.Context()

	userID, ok := auth.UserIDFromContext(ctx)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	orders, err := router.Services.Orders.GetOrdersByID(ctx, userID)
	if err != nil {
		// A failed read is the server's problem, not a malformed request —
		// there is nothing in a GET with no parameters for the client to fix.
		writeError(w, http.StatusInternalServerError, "could not load orders")
		return
	}

	writeJSON(w, http.StatusOK, orders)

}

func (router *OrderRouter) CreateOrder(w http.ResponseWriter, r *http.Request) {
	// POST /trades - Create a new trade
	// Responses:
	// 202 Accepted - Trade request submitted successfully
	// 400 Bad Request - invalid request payload
	// 401 Unauthorized - user not authenticated
	// 404 Not Found - unknown market
	// 503 Service Unavailable - trade queue is full

	ctx := r.Context()

	userID, ok := auth.UserIDFromContext(ctx)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var payload struct {
		Market   string `json:"market"`
		Side     string `json:"side"`
		Quantity int64  `json:"quantity"`
		Price    int64  `json:"price"`
	}

	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if payload.Market == "" {
		http.Error(w, "market is required", http.StatusBadRequest)
		return
	}

	orderID, err := router.Services.Orders.CreateOrder(ctx, userID, payload)
	if err != nil {
		// handleTradeError(w, err)
		// TODO: Improve errors here
		http.Error(w, "Failed to place order", http.StatusBadRequest)
		return
	}

	writeJSON(w, http.StatusAccepted, map[string]any{
		"status":   "accepted",
		"order_id": orderID,
		"market":   payload.Market,
		// "type":       payload.Type,
		"receivedAt": time.Now().UTC().Format(time.RFC3339),
	})

}
