package services

import (
	"context"
	"errors"

	"github.com/ADHFMZ7/crypto-exchange/internal/market"
	"github.com/ADHFMZ7/crypto-exchange/internal/models"
	"github.com/ADHFMZ7/crypto-exchange/internal/orderbook"
	"github.com/ADHFMZ7/crypto-exchange/internal/stores"
)

// TODO: Put proper errrors

type OrderService struct {
	WalletStore *stores.WalletStore
	OrderStore  *stores.OrderStore

	Registry *market.Registry

	Orderbooks map[string]*orderbook.Orderbook
	RQueues    map[string]chan Request
	SChan      chan models.LedgerEvent
}

func NewOrderService(walletStore *stores.WalletStore, orderStore *stores.OrderStore, registry *market.Registry, SChan chan models.LedgerEvent) *OrderService {

	channels := map[string]chan Request{}

	service := &OrderService{
		WalletStore: walletStore,
		OrderStore:  orderStore,

		Registry: registry,

		Orderbooks: make(map[string]*orderbook.Orderbook),
		RQueues:    channels,
		SChan:      SChan,
	}

	for _, m := range registry.Markets() {
		service.Orderbooks[m.Symbol] = orderbook.NewOrderbook()
		channels[m.Symbol] = make(chan Request, 1024)
		go service.StartWorker(channels[m.Symbol], m.Symbol)
	}

	return service
}

func (service *OrderService) StartWorker(channel chan Request, market string) {

	book, ok := service.Orderbooks[market]
	if !ok {
		return
	}

	for request := range channel {

		id := orderbook.OrderID(request.OrderID)

		// Reads are served from this goroutine like every write, because it is
		// the only one that may touch the book. A handler reading the slices
		// directly would be a data race against an in-flight match.
		if request.Type == BookDepth {
			bids, asks := book.Depth(request.Levels)
			request.Reply <- DepthSnapshot{Market: market, Bids: bids, Asks: asks}
			continue
		}

		if request.Type == Cancel {
			book.Cancel(id)
			// Published from here, behind any fills this order already
			// produced, so the ledger releases what is left of the lock and
			// not what an earlier fill still owed.
			service.SChan <- models.LedgerEvent{Cancel: &models.OrderCancel{
				Market:  market,
				OrderID: request.OrderID,
				Side:    request.Side,
			}}
			continue
		}

		shares := orderbook.Shares(request.Shares)
		price := orderbook.Price(request.Price)
		var trades []orderbook.Trade
		var side string

		switch request.Type {
		case LimitBuy:
			trades = book.LimitBuy(id, shares, price)
			side = "buy"
		case LimitSell:
			trades = book.LimitSell(id, shares, price)
			side = "sell"
		default:
			continue
		}

		for _, trade := range trades {
			trade_model := models.Trade{
				Market:          market,
				RestingOrderID:  int64(trade.RestingOrderID),
				IncomingOrderID: int64(trade.IncomingOrderID),
				IncomingSide:    side,
				Quantity:        int64(trade.Quantity),
				Price:           int64(trade.Price),
				ExecutionTime:   trade.ExecutionTime,
			}

			service.SChan <- models.LedgerEvent{Fill: &trade_model}
		}

	}

}

// TODO: Find a place for this later
type OrderReq struct {
	Market   string `json:"market"`
	Side     string `json:"side"`
	Quantity int64  `json:"quantity"`
	Price    int64  `json:"price"`
}

func (service *OrderService) CreateOrder(ctx context.Context, userID int64, payload OrderReq) (int64, error) {

	m, ok := service.Registry.BySymbol(payload.Market)
	if !ok {
		return 0, errors.New("Market does not exist")
	}

	// BTC-USD
	// BTC is base
	// USD is quote

	// If we are doing a buy order, then we are	buying BTC
	// If we are doing a sell order, then we are selling BTC.
	// Price in both is cents per btc

	currency, amount, err := m.Spends(payload.Side, payload.Quantity, payload.Price)
	if err != nil {
		return 0, err
	}

	request_type, err := requestTypeFor(payload.Side)
	if err != nil {
		return 0, err
	}

	req_chan, ok := service.RQueues[m.Symbol]
	if !ok {
		return 0, errors.New("Channel does not exist")
	}

	order_id, err := service.WalletStore.PlaceOrder(ctx, userID, currency.Code, amount, payload.Quantity, payload.Price, payload.Side, m.Symbol)
	if err != nil {
		return 0, err
	}

	// // Consumed by orderbook worker
	// req_chan <- Request{
	// 	Type:    request_type,
	// 	OrderID: order_id,
	// 	Price:   payload.Price,
	// 	Shares:  payload.Quantity,
	// }

	select {
	case req_chan <- Request{
		Type:    request_type,
		OrderID: order_id,
		Price:   payload.Price,
		Shares:  payload.Quantity,
	}:
		return order_id, nil

	case <-ctx.Done():
		// Client is gone. Release the lock rather than leaving an order nobody knows about.
		// TODO: Implement ReleaseOrder in wallet store
		// if err := service.WalletStore.ReleaseOrder(ctx, order_id); err != nil {
		// 	log.Printf("orphaned order %d: funds locked, not queued: %v", order_id, err)
		// }
		return 0, ctx.Err()
	}

}

func (service *OrderService) GetOrdersByID(ctx context.Context, userID int64) (*models.Orders, error) {

	orders, err := service.OrderStore.GetByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}

	return orders, nil
}

type RequestType int

const (
	LimitBuy RequestType = iota
	LimitSell
	Cancel
	BookDepth
)

type Request struct {
	Type    RequestType
	OrderID int64
	Price   int64
	Shares  int64
	Side    string // cancel only: which currency the lock is held in

	// BookDepth only. Reply must be buffered: a caller that gives up waiting
	// would otherwise leave the worker blocked on a send nobody will receive,
	// which stops matching for the whole market.
	Reply  chan DepthSnapshot
	Levels int
}

// DepthSnapshot is one book as the worker saw it, taken between two requests so
// it is never a half-applied match.
type DepthSnapshot struct {
	Market string
	Bids   []orderbook.DepthLevel
	Asks   []orderbook.DepthLevel
}

func requestTypeFor(side string) (RequestType, error) {
	switch side {
	case "buy":
		return LimitBuy, nil
	case "sell":
		return LimitSell, nil
	}
	return 0, market.ErrInvalidSide
}

// func parseOrderType(raw string) (RequestType, bool) {
// 	switch strings.ToLower(strings.TrimSpace(raw)) {
// 	case "limit_buy", "buy":
// 		return LimitBuy, true
// 	case "limit_sell", "sell":
// 		return LimitSell, true
// 	case "cancel":
// 		return Cancel, true
// 	default:
// 		return LimitBuy, false
// 	}
// }

// Errors the order path returns that a handler needs to tell apart, because
// each maps to a different status code.
var (
	ErrUnknownMarket       = errors.New("unknown market")
	ErrOrderNotFound       = errors.New("order not found")
	ErrOrderNotCancellable = errors.New("order is no longer open")
)

// CancelOrder asks the book to stop matching one of the caller's orders.
//
// Ownership and status are checked here, against the ledger, so a caller cannot
// cancel an order that is not theirs. Both are advisory by the time the worker
// acts on the request: an order can fill in the gap, and then the cancellation
// simply loses. That race is settled in one place — the conditional UPDATE in
// WalletStore.CancelOrder — rather than guessed at here.
//
// Returns once the request is queued, not once the book has acted. Cancellation
// is as asynchronous as placement, and for the same reason: the worker owns the
// book, and blocking an HTTP handler on its queue depth helps nobody.
func (service *OrderService) CancelOrder(ctx context.Context, userID, orderID int64) error {

	order, err := service.OrderStore.GetByIDForUser(ctx, orderID, userID)
	if err != nil {
		return err
	}

	if order.Status != models.OrderOpen && order.Status != models.OrderPartiallyFilled {
		return ErrOrderNotCancellable
	}

	queue, ok := service.RQueues[order.Market]
	if !ok {
		return ErrUnknownMarket
	}

	select {
	case queue <- Request{Type: Cancel, OrderID: orderID, Side: order.Side}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Depth returns the resting bids and asks for one market, best first.
//
// The snapshot is taken by the market's own worker, so it is consistent: never
// a book caught midway through applying a match.
func (service *OrderService) Depth(ctx context.Context, symbol string, levels int) (DepthSnapshot, error) {

	m, ok := service.Registry.BySymbol(symbol)
	if !ok {
		return DepthSnapshot{}, ErrUnknownMarket
	}

	queue, ok := service.RQueues[m.Symbol]
	if !ok {
		return DepthSnapshot{}, ErrUnknownMarket
	}

	// Buffered, so abandoning the wait below cannot strand the worker on a send.
	reply := make(chan DepthSnapshot, 1)

	select {
	case queue <- Request{Type: BookDepth, Reply: reply, Levels: levels}:
	case <-ctx.Done():
		return DepthSnapshot{}, ctx.Err()
	}

	select {
	case snapshot := <-reply:
		return snapshot, nil
	case <-ctx.Done():
		return DepthSnapshot{}, ctx.Err()
	}
}
