package services

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

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

	// Build every book and queue before starting any worker, and keep these two
	// loops separate.
	//
	// A worker's first act is to look its own book up in Orderbooks. Starting
	// them inside the loop that fills the map means goroutine N reads it while
	// iteration N+1 writes it — a concurrent map read and write, which Go
	// detects and may simply panic on. With one market the body ran once and
	// nothing wrote after the read, which is the only reason this was survivable
	// before there was a second market to list.
	//
	// After this constructor returns, the maps are never written again, so the
	// workers and every request goroutine can read them without a lock.
	for _, m := range registry.Markets() {
		service.Orderbooks[m.Symbol] = orderbook.NewOrderbook()
		channels[m.Symbol] = make(chan Request, 1024)
	}

	for _, m := range registry.Markets() {
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

	select {
	case req_chan <- Request{
		Type:    request_type,
		OrderID: order_id,
		Price:   payload.Price,
		Shares:  payload.Quantity,
	}:
		return order_id, nil

	case <-ctx.Done():
		// The order is committed and its funds are locked, but the book never
		// saw it, so nothing can ever match it. Left alone it holds those funds
		// until the next restart's flush.
		//
		// The release runs on a fresh context on purpose: ctx is the reason we
		// are here, and handing a cancelled context to the database would fail
		// the release for exactly the same reason it failed the send.
		release, cancel := context.WithTimeout(context.Background(), releaseTimeout)
		defer cancel()

		if err := service.WalletStore.CancelOrder(release, stores.NoEvent, order_id, currency.Code); err != nil {
			log.Printf("orphaned order %d: %d %s locked and never queued: %v",
				order_id, amount, currency.Code, err)
		}

		return 0, ctx.Err()
	}

}

// How long to spend releasing an order the client abandoned. Short: the caller
// is already gone, and the startup flush is the backstop if this does not land.
const releaseTimeout = 5 * time.Second

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

// CancelRestingOrders unwinds every order left resting by a previous run.
//
// The book is in memory and Postgres is not, so a restart leaves rows that
// nothing can ever match, holding funds against orders that no longer exist
// anywhere. Until the book is rebuilt from the ledger instead, cancelling them
// is what makes the two agree: it cannot be wrong, it needs no ordering
// guarantees, and it costs one transaction.
//
// It must run before the server accepts anything. An order placed against a
// market that is halfway through its flush would rest beside orders about to be
// cancelled, which is the inconsistency this exists to remove.
func (service *OrderService) CancelRestingOrders(ctx context.Context) error {

	resting, err := service.OrderStore.RestingOrders(ctx)
	if err != nil {
		return fmt.Errorf("reading resting orders: %w", err)
	}
	if len(resting) == 0 {
		return nil
	}

	releases := make([]stores.OrderRelease, 0, len(resting))
	for _, order := range resting {
		m, ok := service.Registry.BySymbol(order.Market)
		if !ok {
			// A market the registry no longer lists. Its lock cannot be
			// returned without knowing which currency it is in, and guessing
			// would move the wrong money.
			return fmt.Errorf("order %d rests on unknown market %q", order.ID, order.Market)
		}

		currency, err := m.Locks(order.Side)
		if err != nil {
			return fmt.Errorf("order %d: %w", order.ID, err)
		}

		releases = append(releases, stores.OrderRelease{OrderID: order.ID, Currency: currency.Code})
	}

	freed, err := service.WalletStore.CancelRestingOrders(ctx, releases)
	if err != nil {
		return err
	}

	for currency, amount := range freed {
		log.Printf("startup: released %d %s minor units held by orders from a previous run",
			amount, currency)
	}
	log.Printf("startup: cancelled %d resting order(s) the in-memory book no longer holds", len(resting))

	return nil
}
