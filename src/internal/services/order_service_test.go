package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ADHFMZ7/crypto-exchange/internal/market"
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

// newTestService wires a registry and a queue per market, with no store behind
// it. The queue is buffered and never drained, so nothing consumes what
// CreateOrder enqueues.
func newTestService(t *testing.T) *OrderService {
	t.Helper()

	registry := testRegistry(t)
	queues := map[string]chan Request{}
	for _, m := range registry.Markets() {
		queues[m.Symbol] = make(chan Request, 8)
	}

	return &OrderService{
		Registry:  registry,
		Orderbook: orderbook.NewOrderbook(),
		RQueues:   queues,
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

func TestStartWorkerAppliesRequestsToTheBook(t *testing.T) {
	service := &OrderService{Orderbook: orderbook.NewOrderbook()}

	// Buffered and closed up front, so StartWorker drains and returns rather
	// than blocking — no goroutine, no sleep, fully deterministic.
	queue := make(chan Request, 4)
	queue <- Request{Type: LimitSell, OrderID: 1, Shares: 100, Price: 2400}
	queue <- Request{Type: LimitBuy, OrderID: 2, Shares: 40, Price: 2500}
	close(queue)

	service.StartWorker(queue)

	// The buy crossed and took 40 of the resting 100.
	if got := service.Orderbook.BestSell(); got != 2400 {
		t.Fatalf("best ask = %d, want 2400", got)
	}
	if got := service.Orderbook.BestBuy(); got != -1 {
		t.Fatalf("best bid = %d, want -1: the buy was fully filled", got)
	}

	level := service.Orderbook.LevelsSell[service.Orderbook.LevelMapSell[2400]]
	resting, ok := level.Orders.Peek()
	if !ok {
		t.Fatal("expected a resting sell order")
	}
	if resting.Shares != 60 {
		t.Fatalf("resting shares = %d, want 60", resting.Shares)
	}
}

func TestStartWorkerHandlesCancel(t *testing.T) {
	service := &OrderService{Orderbook: orderbook.NewOrderbook()}

	queue := make(chan Request, 4)
	queue <- Request{Type: LimitBuy, OrderID: 1, Shares: 100, Price: 2500}
	queue <- Request{Type: Cancel, OrderID: 1}
	close(queue)

	service.StartWorker(queue)

	// Cancellation is lazy: the order is flagged, and evicted when matching
	// next reaches it. So the incoming sell finds nothing to trade with.
	service.Orderbook.LimitSell(2, 50, 2400)

	if got := service.Orderbook.BestBuy(); got != -1 {
		t.Fatalf("best bid = %d, want -1: the only bid was cancelled", got)
	}
	if got := service.Orderbook.BestSell(); got != 2400 {
		t.Fatalf("best ask = %d, want 2400: the sell should rest, not fill", got)
	}
}

// StartWorker returns when its channel closes. Anything else leaks a goroutine
// per market for the lifetime of the process.
func TestStartWorkerReturnsWhenChannelCloses(t *testing.T) {
	service := &OrderService{Orderbook: orderbook.NewOrderbook()}

	queue := make(chan Request)
	close(queue)

	done := make(chan struct{})
	go func() {
		service.StartWorker(queue)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("StartWorker did not return after its channel closed: goroutine leaked")
	}
}
