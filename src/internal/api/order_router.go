package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/ADHFMZ7/crypto-exchange/internal/auth"
	"github.com/ADHFMZ7/crypto-exchange/internal/market"
	"github.com/ADHFMZ7/crypto-exchange/internal/services"
	"github.com/ADHFMZ7/crypto-exchange/internal/stores"
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
	mux.Handle(
		"DELETE /orders/{id}",
		Authenticate(http.HandlerFunc(router.CancelOrder)),
	)
}

// CancelOrder asks the book to stop matching one of the caller's orders.
//
// 202, not 200: the book is owned by a worker goroutine and the funds are
// released by the settlement worker behind it, so a successful response means
// the request is queued and ordered behind any fills already in flight — not
// that the order is cancelled yet. Poll GET /orders for the outcome, exactly as
// with placement.
//
// The order can still fill in that window. That is not an error the client can
// avoid by retrying, and the status will simply read `filled`.
func (router *OrderRouter) CancelOrder(w http.ResponseWriter, r *http.Request) {

	ctx := r.Context()

	userID, ok := auth.UserIDFromContext(ctx)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	orderID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || orderID <= 0 {
		writeError(w, http.StatusBadRequest, "order id must be a positive integer")
		return
	}

	err = router.Services.Orders.CancelOrder(ctx, userID, orderID)

	switch {
	case err == nil:
		writeJSON(w, http.StatusAccepted, map[string]any{
			"status":   "cancelling",
			"order_id": orderID,
		})

	case errors.Is(err, stores.ErrOrderNotFound):
		// Also covers another user's order: see stores.ErrOrderNotFound.
		writeError(w, http.StatusNotFound, "order not found")

	case errors.Is(err, services.ErrOrderNotCancellable):
		writeError(w, http.StatusConflict, "order is no longer open")

	case errors.Is(err, services.ErrUnknownMarket):
		writeError(w, http.StatusNotFound, "unknown market")

	default:
		writeError(w, http.StatusInternalServerError, "could not cancel order")
	}
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
