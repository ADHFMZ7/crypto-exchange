package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ADHFMZ7/crypto-exchange/internal/market"
	"github.com/ADHFMZ7/crypto-exchange/internal/models"
	"github.com/ADHFMZ7/crypto-exchange/internal/orderbook"
)

/*
OrderService tests that need no database.

CreateOrder validates everything — market, side, amounts, queue — before it
calls WalletStore.PlaceOrder, which is the only line that touches Postgres.
That ordering is deliberate (nothing is committed until every precondition
holds), and it is what lets these tests run with a nil WalletStore: a case that
slipped past the guards would panic rather than quietly pass.

Tests build OrderService as a struct literal rather than through
NewOrderService, which starts a worker goroutine per market that would outlive
the test.
*/

func testRegistry(t *testing.T) *market.Registry {
	t.Helper()

	registry, err := market.NewMarketRegistry(market.Default())
	if err != nil {
		t.Fatal(err)
	}
	return registry
}

// newTestService wires a registry, a queue and a book per market, with no store
// behind it. The queue is buffered and never drained, so nothing consumes what
// CreateOrder enqueues.
func newTestService(t *testing.T) *OrderService {
	t.Helper()

	registry := testRegistry(t)
	queues := map[string]chan Request{}
	books := map[string]*orderbook.Orderbook{}
	for _, m := range registry.Markets() {
		queues[m.Symbol] = make(chan Request, 8)
		books[m.Symbol] = orderbook.NewOrderbook()
	}

	return &OrderService{
		Registry:   registry,
		Orderbooks: books,
		RQueues:    queues,
	}
}

func TestRequestTypeFor(t *testing.T) {
	valid := map[string]RequestType{
		"buy":  LimitBuy,
		"sell": LimitSell,
	}

	for side, want := range valid {
		got, err := requestTypeFor(side)
		if err != nil {
			t.Fatalf("requestTypeFor(%q) returned %v", side, err)
		}
		if got != want {
			t.Fatalf("requestTypeFor(%q) = %v, want %v", side, got, want)
		}
	}

	// Matching is exact, so anything else must fail rather than default to a
	// side. Note "cancel" is no longer an order side — cancellation is its own
	// operation now.
	for _, side := range []string{"", "Buy", "SELL", "limit_buy", "cancel", "hold"} {
		if _, err := requestTypeFor(side); !errors.Is(err, market.ErrInvalidSide) {
			t.Fatalf("requestTypeFor(%q) err = %v, want ErrInvalidSide", side, err)
		}
	}
}

// requestTypeFor and Market.Spends both decide what a side means. If they ever
// disagree, an order locks one currency and enters the book on the other side.
func TestSideIsInterpretedConsistentlyBySpendsAndTheQueue(t *testing.T) {
	registry := testRegistry(t)
	m, ok := registry.BySymbol("BTC-USD")
	if !ok {
		t.Fatal("BTC-USD missing from the default registry")
	}

	cases := []struct {
		side         string
		wantType     RequestType
		wantCurrency string
	}{
		{side: "buy", wantType: LimitBuy, wantCurrency: "USD"},
		{side: "sell", wantType: LimitSell, wantCurrency: "BTC"},
	}

	for _, tc := range cases {
		t.Run(tc.side, func(t *testing.T) {
			requestType, err := requestTypeFor(tc.side)
			if err != nil {
				t.Fatal(err)
			}
			currency, _, err := m.Spends(tc.side, 10_000_000, 4_500_000)
			if err != nil {
				t.Fatal(err)
			}

			if requestType != tc.wantType {
				t.Fatalf("queue type = %v, want %v", requestType, tc.wantType)
			}
			if currency.Code != tc.wantCurrency {
				t.Fatalf("%s debits %s, want %s", tc.side, currency.Code, tc.wantCurrency)
			}
		})
	}
}

func TestCreateOrderRejectsBeforeTouchingTheStore(t *testing.T) {
	service := newTestService(t)

	cases := []struct {
		name    string
		payload OrderReq
	}{
		{
			name:    "unknown market",
			payload: OrderReq{Market: "DOGE-USD", Side: "buy", Quantity: 1, Price: 1},
		},
		{
			name:    "empty market",
			payload: OrderReq{Market: "", Side: "buy", Quantity: 1, Price: 1},
		},
		{
			name:    "unknown side",
			payload: OrderReq{Market: "BTC-USD", Side: "teleport", Quantity: 1, Price: 1},
		},
		{
			name:    "empty side",
			payload: OrderReq{Market: "BTC-USD", Side: "", Quantity: 1, Price: 1},
		},
		{
			// The wire format is case-sensitive; this documents that.
			name:    "wrong case side",
			payload: OrderReq{Market: "BTC-USD", Side: "Buy", Quantity: 1, Price: 1},
		},
		{
			name:    "zero quantity",
			payload: OrderReq{Market: "BTC-USD", Side: "buy", Quantity: 0, Price: 4_500_000},
		},
		{
			name:    "negative quantity",
			payload: OrderReq{Market: "BTC-USD", Side: "buy", Quantity: -1, Price: 4_500_000},
		},
		{
			name:    "zero price",
			payload: OrderReq{Market: "BTC-USD", Side: "buy", Quantity: 10_000_000, Price: 0},
		},
		{
			name:    "negative price",
			payload: OrderReq{Market: "BTC-USD", Side: "buy", Quantity: 10_000_000, Price: -1},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// A nil WalletStore is the assertion: reaching PlaceOrder panics.
			orderID, err := service.CreateOrder(context.Background(), 42, tc.payload)
			if err == nil {
				t.Fatalf("accepted an invalid order, id %d", orderID)
			}
			if orderID != 0 {
				t.Fatalf("rejected order returned id %d, want 0", orderID)
			}
		})
	}

	// Nothing invalid should have been queued.
	for symbol, queue := range service.RQueues {
		if len(queue) != 0 {
			t.Fatalf("%s queue holds %d requests after only invalid orders", symbol, len(queue))
		}
	}
}

// A market listed in the registry but missing a queue would be a send on a nil
// channel, which blocks the request goroutine forever. CreateOrder must catch
// it — and must do so before locking any funds.
func TestCreateOrderRejectsMarketWithNoQueue(t *testing.T) {
	service := newTestService(t)
	service.RQueues = map[string]chan Request{} // registry still lists BTC-USD

	orderID, err := service.CreateOrder(context.Background(), 42, OrderReq{
		Market:   "BTC-USD",
		Side:     "buy",
		Quantity: 10_000_000,
		Price:    4_500_000,
	})
	if err == nil {
		t.Fatalf("accepted an order for a market with no queue, id %d", orderID)
	}
}

// newWorkerService returns a service holding one book for symbol, that book, and
// the channel the worker publishes fills to, so a test can drive the worker and
// then inspect both what it did and what it reported.
//
// The channel must be buffered past anything the test enqueues. StartWorker
// publishes each fill before it takes the next request, so with nothing draining
// the other end a full channel blocks the worker — and the test — forever.
func newWorkerService(symbol string) (*OrderService, *orderbook.Orderbook, chan models.LedgerEvent) {
	book := orderbook.NewOrderbook()
	settlements := make(chan models.LedgerEvent, 16)
	return &OrderService{
		Orderbooks: map[string]*orderbook.Orderbook{symbol: book},
		SChan:      settlements,
	}, book, settlements
}

// drainSettlements closes the channel and collects everything on it. Safe only
// once StartWorker has returned, which means it has no more fills to publish.
func drainSettlements(settlements chan models.LedgerEvent) []models.LedgerEvent {
	close(settlements)

	var events []models.LedgerEvent
	for event := range settlements {
		events = append(events, event)
	}
	return events
}

func TestStartWorkerAppliesRequestsToTheBook(t *testing.T) {
	service, book, settlements := newWorkerService("BTC-USD")

	// Buffered and closed up front, so StartWorker drains and returns rather
	// than blocking — no goroutine, no sleep, fully deterministic. The
	// settlement channel is buffered for the same reason: the worker publishes
	// the fill this crossing produces before it can return.
	queue := make(chan Request, 4)
	queue <- Request{Type: LimitSell, OrderID: 1, Shares: 100, Price: 2400}
	queue <- Request{Type: LimitBuy, OrderID: 2, Shares: 40, Price: 2500}
	close(queue)

	service.StartWorker(queue, "BTC-USD")

	// The buy crossed and took 40 of the resting 100.
	if got := book.BestSell(); got != 2400 {
		t.Fatalf("best ask = %d, want 2400", got)
	}
	if got := book.BestBuy(); got != -1 {
		t.Fatalf("best bid = %d, want -1: the buy was fully filled", got)
	}

	level := book.LevelsSell[book.LevelMapSell[2400]]
	resting, ok := level.Orders.Peek()
	if !ok {
		t.Fatal("expected a resting sell order")
	}
	if resting.Shares != 60 {
		t.Fatalf("resting shares = %d, want 60", resting.Shares)
	}

	// The crossing has to reach settlement, or the book has moved and the
	// ledger will never hear about it.
	events := drainSettlements(settlements)
	if len(events) != 1 {
		t.Fatalf("published %d events, want 1", len(events))
	}
	if events[0].Fill == nil {
		t.Fatalf("published %+v, want a fill", events[0])
	}

	fill := *events[0].Fill
	if fill.Market != "BTC-USD" {
		t.Errorf("fill market = %q, want BTC-USD", fill.Market)
	}
	if fill.IncomingSide != "buy" {
		t.Errorf("fill incoming side = %q, want buy: the buy crossed", fill.IncomingSide)
	}
	if fill.RestingOrderID != 1 || fill.IncomingOrderID != 2 {
		t.Errorf("fill orders = resting %d, incoming %d, want 1 and 2",
			fill.RestingOrderID, fill.IncomingOrderID)
	}
	if fill.Quantity != 40 {
		t.Errorf("fill quantity = %d, want 40", fill.Quantity)
	}
	// The resting sell's price, not the buyer's 2500 limit. Settlement refunds
	// the difference, so reporting the taker's limit here would silently
	// overcharge the buyer.
	if fill.Price != 2400 {
		t.Errorf("fill price = %d, want 2400", fill.Price)
	}
	if fill.ExecutionTime.IsZero() {
		t.Error("fill execution time is zero: it is what orders the tape")
	}
}

func TestStartWorkerHandlesCancel(t *testing.T) {
	service, book, _ := newWorkerService("BTC-USD")

	queue := make(chan Request, 4)
	queue <- Request{Type: LimitBuy, OrderID: 1, Shares: 100, Price: 2500}
	queue <- Request{Type: Cancel, OrderID: 1}
	close(queue)

	service.StartWorker(queue, "BTC-USD")

	// Cancellation is lazy: the order is flagged, and evicted when matching
	// next reaches it. So the incoming sell finds nothing to trade with.
	book.LimitSell(2, 50, 2400)

	if got := book.BestBuy(); got != -1 {
		t.Fatalf("best bid = %d, want -1: the only bid was cancelled", got)
	}
	if got := book.BestSell(); got != 2400 {
		t.Fatalf("best ask = %d, want 2400: the sell should rest, not fill", got)
	}
}

// Each market owns its own book, so an order on one market must not match
// against orders resting on another. This is the whole point of issue #4:
// before it, one Orderbook was shared by every market's worker.
func TestMarketsDoNotShareABook(t *testing.T) {
	btcBook := orderbook.NewOrderbook()
	ethBook := orderbook.NewOrderbook()

	service := &OrderService{
		Orderbooks: map[string]*orderbook.Orderbook{
			"BTC-USD": btcBook,
			"ETH-USD": ethBook,
		},
		SChan: make(chan models.LedgerEvent, 16),
	}

	btcQueue := make(chan Request, 2)
	btcQueue <- Request{Type: LimitSell, OrderID: 1, Shares: 100, Price: 2400}
	close(btcQueue)
	service.StartWorker(btcQueue, "BTC-USD")

	// Priced far above the resting BTC ask. On a shared book it would cross it.
	ethQueue := make(chan Request, 2)
	ethQueue <- Request{Type: LimitBuy, OrderID: 2, Shares: 100, Price: 9999}
	close(ethQueue)
	service.StartWorker(ethQueue, "ETH-USD")

	if got := btcBook.BestSell(); got != 2400 {
		t.Fatalf("BTC best ask = %d, want 2400: the ETH buy took liquidity from another market", got)
	}
	if got := ethBook.BestBuy(); got != 9999 {
		t.Fatalf("ETH best bid = %d, want 9999: it should rest, not fill", got)
	}
	if got := ethBook.BestSell(); got != -1 {
		t.Fatalf("ETH best ask = %d, want -1: nothing was ever sold on this market", got)
	}
}

// A worker started for a market with no book must return rather than run on
// with a nil book, which would panic on the first request.
func TestStartWorkerReturnsWhenMarketHasNoBook(t *testing.T) {
	service := &OrderService{Orderbooks: map[string]*orderbook.Orderbook{}}

	queue := make(chan Request, 1)
	queue <- Request{Type: LimitBuy, OrderID: 1, Shares: 100, Price: 2500}

	done := make(chan struct{})
	go func() {
		service.StartWorker(queue, "BTC-USD")
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("StartWorker did not return for a market with no book")
	}

	if len(queue) != 1 {
		t.Fatal("StartWorker consumed a request for a market it has no book for")
	}
}

// StartWorker returns when its channel closes. Anything else leaks a goroutine
// per market for the lifetime of the process.
func TestStartWorkerReturnsWhenChannelCloses(t *testing.T) {
	// The book must exist, or this passes for the wrong reason — the worker
	// would return on the missing-book guard without ever reaching the range.
	service, _, _ := newWorkerService("BTC-USD")

	queue := make(chan Request)
	close(queue)

	done := make(chan struct{})
	go func() {
		service.StartWorker(queue, "BTC-USD")
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("StartWorker did not return after its channel closed: goroutine leaked")
	}
}

// A cancellation must reach the ledger, or the order stops matching in memory
// while its funds stay locked in Postgres forever.
func TestStartWorkerPublishesCancellations(t *testing.T) {
	service, book, settlements := newWorkerService("BTC-USD")

	queue := make(chan Request, 4)
	queue <- Request{Type: LimitSell, OrderID: 7, Shares: 100, Price: 2400}
	queue <- Request{Type: Cancel, OrderID: 7, Side: "sell"}
	close(queue)

	service.StartWorker(queue, "BTC-USD")

	events := drainSettlements(settlements)
	if len(events) != 1 {
		t.Fatalf("published %d events, want 1", len(events))
	}
	cancel := events[0].Cancel
	if cancel == nil {
		t.Fatalf("published %+v, want a cancellation", events[0])
	}
	if cancel.OrderID != 7 || cancel.Market != "BTC-USD" || cancel.Side != "sell" {
		t.Fatalf("cancellation = %+v, want order 7, BTC-USD, sell", *cancel)
	}

	// The side is what tells the ledger which currency to give back, so it has
	// to survive the trip rather than be re-derived downstream.
	if _, asks := book.Depth(0); len(asks) != 0 {
		t.Fatalf("asks = %+v, want the cancelled order gone from depth", asks)
	}
}

// Fills and cancellations share a queue so they stay ordered. A cancellation
// that overtook its own fill would release a lock the fill still needs.
func TestStartWorkerKeepsFillsAheadOfTheCancelThatFollows(t *testing.T) {
	service, _, settlements := newWorkerService("BTC-USD")

	queue := make(chan Request, 8)
	queue <- Request{Type: LimitSell, OrderID: 1, Shares: 100, Price: 2400}
	queue <- Request{Type: LimitBuy, OrderID: 2, Shares: 40, Price: 2500} // fills 40
	queue <- Request{Type: Cancel, OrderID: 1, Side: "sell"}              // cancels the rest
	close(queue)

	service.StartWorker(queue, "BTC-USD")

	events := drainSettlements(settlements)
	if len(events) != 2 {
		t.Fatalf("published %d events, want 2", len(events))
	}
	if events[0].Fill == nil {
		t.Fatalf("first event = %+v, want the fill", events[0])
	}
	if events[1].Cancel == nil {
		t.Fatalf("second event = %+v, want the cancellation", events[1])
	}
}
