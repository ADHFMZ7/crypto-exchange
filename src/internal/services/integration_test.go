package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ADHFMZ7/crypto-exchange/internal/market"
	"github.com/ADHFMZ7/crypto-exchange/internal/models"
	"github.com/ADHFMZ7/crypto-exchange/internal/orderbook"
	"github.com/ADHFMZ7/crypto-exchange/internal/stores"
	"github.com/ADHFMZ7/crypto-exchange/internal/stream"
	"github.com/ADHFMZ7/crypto-exchange/internal/testsupport"
	"github.com/jackc/pgx/v5/pgxpool"
)

/*
Service-level tests against a real Postgres.

The no-database tests above cover what the order path decides. These cover what
it does: which currency it locks and how much, what it leaves behind when
something goes wrong, and whether the two halves of the exchange still agree
afterwards.

They skip without TEST_DATABASE_URL, like every other integration test here.
*/

const (
	oneBTC       = int64(100_000_000)
	fiftyDollars = int64(5_000)
	symbol       = "BTC-USD"
)

type harness struct {
	pool     *pgxpool.Pool
	orders   *OrderService
	trades   *TradeService
	wallets  *stores.WalletStore
	outbox   *stores.OutboxStore
	registry *market.Registry
	events   chan models.LedgerEvent
	hub      *stream.Hub
}

// newHarness wires the real services over an empty schema, with no goroutines
// running: a test drives the worker itself so nothing happens between an action
// and the assertion about it.
func newHarness(t *testing.T) *harness {
	t.Helper()

	pool := testsupport.Pool(t, "services")
	testsupport.Truncate(t, pool)

	registry, err := market.NewMarketRegistry(market.Default())
	if err != nil {
		t.Fatal(err)
	}

	all := stores.NewStores(pool)
	events := make(chan models.LedgerEvent, 64)

	hub := stream.NewHub()

	orders := &OrderService{
		WalletStore: all.Wallets,
		Stream:      hub,
		OrderStore:  all.Orders,
		Registry:    registry,
		Orderbooks:  map[string]*orderbook.Orderbook{},
		RQueues:     map[string]chan Request{},
		SChan:       events,
	}
	for _, m := range registry.Markets() {
		orders.Orderbooks[m.Symbol] = orderbook.NewOrderbook()
		orders.RQueues[m.Symbol] = make(chan Request, 64)
	}

	trades := &TradeService{
		WalletStore:    all.Wallets,
		Stream:         hub,
		UserStore:      all.Users,
		TradeStore:     all.Trades,
		OutboxStore:    all.Outbox,
		MarketRegistry: registry,
		SettlementChan: events,
	}

	return &harness{
		pool: pool, orders: orders, trades: trades,
		wallets: all.Wallets, outbox: all.Outbox, registry: registry, events: events, hub: hub,
	}
}

func (h *harness) user(t *testing.T, email string, usd, btc int64) int64 {
	t.Helper()

	var id int64
	err := h.pool.QueryRow(context.Background(),
		`INSERT INTO users (fullname, email, hashed_password) VALUES ($1, $1, 'x') RETURNING id`,
		email).Scan(&id)
	if err != nil {
		t.Fatal(err)
	}
	for currency, amount := range map[string]int64{"USD": usd, "BTC": btc} {
		if amount == 0 {
			continue
		}
		if _, err := h.pool.Exec(context.Background(),
			`INSERT INTO balances (user_id, currency, available) VALUES ($1, $2, $3)`,
			id, currency, amount); err != nil {
			t.Fatal(err)
		}
	}
	return id
}

func (h *harness) balance(t *testing.T, user int64, currency string) (available, locked int64) {
	t.Helper()

	err := h.pool.QueryRow(context.Background(),
		`SELECT available, locked FROM balances WHERE user_id = $1 AND currency = $2`,
		user, currency).Scan(&available, &locked)
	if err != nil {
		t.Fatalf("balance %d/%s: %v", user, currency, err)
	}
	return available, locked
}

func (h *harness) order(t *testing.T, id int64) (filled int64, status string, locked int64) {
	t.Helper()

	err := h.pool.QueryRow(context.Background(),
		`SELECT filled_quantity, status, locked_remaining FROM orders WHERE id = $1`, id).
		Scan(&filled, &status, &locked)
	if err != nil {
		t.Fatalf("order %d: %v", id, err)
	}
	return filled, status, locked
}

// drain runs the market's worker over whatever is queued, then stops.
func (h *harness) drain(t *testing.T) {
	t.Helper()

	close(h.orders.RQueues[symbol])
	h.orders.StartWorker(h.orders.RQueues[symbol], symbol)
	h.orders.RQueues[symbol] = make(chan Request, 64)
}

// settle applies every event the worker published.
func (h *harness) settle(t *testing.T) {
	t.Helper()

	for {
		select {
		case event := <-h.events:
			h.trades.absorb(context.Background(), event)
		default:
			return
		}
	}
}

func (h *harness) totals(t *testing.T) map[string]int64 {
	t.Helper()

	rows, err := h.pool.Query(context.Background(),
		`SELECT currency, sum(available + locked) FROM balances GROUP BY currency`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()

	totals := map[string]int64{}
	for rows.Next() {
		var currency string
		var total int64
		if err := rows.Scan(&currency, &total); err != nil {
			t.Fatal(err)
		}
		totals[currency] = total
	}
	return totals
}

// ── order entry ───────────────────────────────────────────────────────────

func TestCreateOrderLocksWhatTheSideSpends(t *testing.T) {
	h := newHarness(t)
	buyer := h.user(t, "buyer@test", 1_000_000, 0)
	seller := h.user(t, "seller@test", 0, oneBTC*5)

	buyID, err := h.orders.CreateOrder(context.Background(), buyer, OrderReq{
		Market: symbol, Side: "buy", Quantity: oneBTC, Price: fiftyDollars,
	})
	if err != nil {
		t.Fatal(err)
	}
	// A buy locks quote: quantity x price, rounded up as the ledger does.
	if available, locked := h.balance(t, buyer, "USD"); locked != fiftyDollars ||
		available != 1_000_000-fiftyDollars {
		t.Errorf("buyer USD = (%d, %d), want (%d, %d)",
			available, locked, 1_000_000-fiftyDollars, fiftyDollars)
	}

	sellID, err := h.orders.CreateOrder(context.Background(), seller, OrderReq{
		Market: symbol, Side: "sell", Quantity: oneBTC, Price: fiftyDollars,
	})
	if err != nil {
		t.Fatal(err)
	}
	// A sell locks base, one for one with the quantity.
	if available, locked := h.balance(t, seller, "BTC"); locked != oneBTC ||
		available != oneBTC*4 {
		t.Errorf("seller BTC = (%d, %d), want (%d, %d)", available, locked, oneBTC*4, oneBTC)
	}

	if buyID == sellID {
		t.Error("two orders share an id")
	}
}

func TestCreateOrderRejectsWhatItShould(t *testing.T) {
	h := newHarness(t)
	user := h.user(t, "trader@test", 1_000_000, oneBTC)

	cases := []struct {
		name string
		req  OrderReq
	}{
		{"unknown market", OrderReq{Market: "NOPE-USD", Side: "buy", Quantity: 1, Price: 1}},
		{"empty market", OrderReq{Market: "", Side: "buy", Quantity: 1, Price: 1}},
		{"unknown side", OrderReq{Market: symbol, Side: "sideways", Quantity: 1, Price: 1}},
		{"empty side", OrderReq{Market: symbol, Side: "", Quantity: 1, Price: 1}},
		{"zero quantity", OrderReq{Market: symbol, Side: "buy", Quantity: 0, Price: 1}},
		{"negative quantity", OrderReq{Market: symbol, Side: "buy", Quantity: -1, Price: 1}},
		{"zero price", OrderReq{Market: symbol, Side: "buy", Quantity: 1, Price: 0}},
		{"negative price", OrderReq{Market: symbol, Side: "buy", Quantity: 1, Price: -1}},
		{"overflowing notional", OrderReq{
			Market: symbol, Side: "buy", Quantity: 1 << 62, Price: 1 << 62}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := h.orders.CreateOrder(context.Background(), user, tc.req); err == nil {
				t.Fatal("accepted an order it should have refused")
			}
		})
	}

	// Nothing was locked by any of them.
	if _, locked := h.balance(t, user, "USD"); locked != 0 {
		t.Errorf("USD locked = %d after only rejected orders", locked)
	}
	if _, locked := h.balance(t, user, "BTC"); locked != 0 {
		t.Errorf("BTC locked = %d after only rejected orders", locked)
	}
}

func TestCreateOrderRefusesWhatTheBalanceCannotCover(t *testing.T) {
	h := newHarness(t)
	user := h.user(t, "trader@test", fiftyDollars-1, 0)

	if _, err := h.orders.CreateOrder(context.Background(), user, OrderReq{
		Market: symbol, Side: "buy", Quantity: oneBTC, Price: fiftyDollars,
	}); err == nil {
		t.Fatal("accepted an order the balance cannot cover")
	}

	if available, locked := h.balance(t, user, "USD"); available != fiftyDollars-1 || locked != 0 {
		t.Errorf("USD = (%d, %d), want it untouched", available, locked)
	}

	var orders int
	h.pool.QueryRow(context.Background(), `SELECT count(*) FROM orders`).Scan(&orders)
	if orders != 0 {
		t.Errorf("%d order rows survived a rejected placement", orders)
	}
}

// The regression for an order that was committed and locked but never reached
// the book. Nothing can match it, so left alone it holds those funds until the
// next restart's flush.
func TestCreateOrderReleasesAnOrderTheClientAbandoned(t *testing.T) {
	h := newHarness(t)
	user := h.user(t, "trader@test", 1_000_000, 0)

	// Unbuffered and unread, so the send blocks and the context decides.
	h.orders.RQueues[symbol] = make(chan Request)

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(30 * time.Millisecond)
		cancel()
	}()

	_, err := h.orders.CreateOrder(ctx, user, OrderReq{
		Market: symbol, Side: "buy", Quantity: oneBTC, Price: fiftyDollars,
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("CreateOrder = %v, want context.Canceled", err)
	}

	available, locked := h.balance(t, user, "USD")
	if locked != 0 {
		t.Errorf("USD locked = %d — an order nothing can match is holding funds", locked)
	}
	if available != 1_000_000 {
		t.Errorf("USD available = %d, want the full %d back", available, 1_000_000)
	}

	var status string
	if err := h.pool.QueryRow(context.Background(),
		`SELECT status FROM orders ORDER BY id DESC LIMIT 1`).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != models.OrderCancelled {
		t.Errorf("order status = %q, want %q", status, models.OrderCancelled)
	}
}

// ── the full path: place, match, settle ───────────────────────────────────

func TestAnOrderPairTradesAndSettles(t *testing.T) {
	h := newHarness(t)
	buyer := h.user(t, "buyer@test", 1_000_000, 0)
	seller := h.user(t, "seller@test", 0, oneBTC*5)

	before := h.totals(t)

	sellID, err := h.orders.CreateOrder(context.Background(), seller, OrderReq{
		Market: symbol, Side: "sell", Quantity: oneBTC, Price: fiftyDollars,
	})
	if err != nil {
		t.Fatal(err)
	}
	// Crosses: a buyer willing to pay more than the resting ask.
	buyID, err := h.orders.CreateOrder(context.Background(), buyer, OrderReq{
		Market: symbol, Side: "buy", Quantity: oneBTC, Price: fiftyDollars * 2,
	})
	if err != nil {
		t.Fatal(err)
	}

	h.drain(t)
	h.settle(t)

	for _, id := range []int64{buyID, sellID} {
		filled, status, locked := h.order(t, id)
		if filled != oneBTC || status != models.OrderFilled || locked != 0 {
			t.Errorf("order %d = (filled %d, %q, locked %d), want fully filled and unlocked",
				id, filled, status, locked)
		}
	}

	// It executed at the resting ask, not the buyer's limit, so the difference
	// came back rather than being spent.
	if available, locked := h.balance(t, buyer, "USD"); available != 1_000_000-fiftyDollars || locked != 0 {
		t.Errorf("buyer USD = (%d, %d), want (%d, 0) — price improvement not returned",
			available, locked, 1_000_000-fiftyDollars)
	}
	if available, _ := h.balance(t, buyer, "BTC"); available != oneBTC {
		t.Errorf("buyer BTC = %d, want %d", available, oneBTC)
	}
	if available, _ := h.balance(t, seller, "USD"); available != fiftyDollars {
		t.Errorf("seller USD = %d, want %d", available, fiftyDollars)
	}

	var trades int
	h.pool.QueryRow(context.Background(), `SELECT count(*) FROM trades`).Scan(&trades)
	if trades != 1 {
		t.Errorf("%d trade rows for one crossing", trades)
	}

	for currency, total := range before {
		if h.totals(t)[currency] != total {
			t.Errorf("%s total moved from %d to %d", currency, total, h.totals(t)[currency])
		}
	}
}

func TestAPartialFillLeavesTheRestResting(t *testing.T) {
	h := newHarness(t)
	buyer := h.user(t, "buyer@test", 1_000_000, 0)
	seller := h.user(t, "seller@test", 0, oneBTC*5)

	sellID, _ := h.orders.CreateOrder(context.Background(), seller, OrderReq{
		Market: symbol, Side: "sell", Quantity: oneBTC, Price: fiftyDollars,
	})
	buyID, _ := h.orders.CreateOrder(context.Background(), buyer, OrderReq{
		Market: symbol, Side: "buy", Quantity: oneBTC / 4, Price: fiftyDollars,
	})

	h.drain(t)
	h.settle(t)

	if filled, status, _ := h.order(t, buyID); filled != oneBTC/4 || status != models.OrderFilled {
		t.Errorf("buy order = (%d, %q), want fully filled at a quarter", filled, status)
	}
	filled, status, locked := h.order(t, sellID)
	if filled != oneBTC/4 || status != models.OrderPartiallyFilled {
		t.Errorf("sell order = (%d, %q), want partially filled", filled, status)
	}
	if locked != oneBTC-oneBTC/4 {
		t.Errorf("sell locked_remaining = %d, want the unsold %d", locked, oneBTC-oneBTC/4)
	}

	// The unsold base is still committed to the resting order.
	if _, lockedBTC := h.balance(t, seller, "BTC"); lockedBTC != oneBTC-oneBTC/4 {
		t.Errorf("seller BTC locked = %d, want %d", lockedBTC, oneBTC-oneBTC/4)
	}
}

// ── cancellation ──────────────────────────────────────────────────────────

func TestCancelOrderReturnsTheLockThroughTheLedger(t *testing.T) {
	h := newHarness(t)
	user := h.user(t, "trader@test", 1_000_000, 0)

	id, err := h.orders.CreateOrder(context.Background(), user, OrderReq{
		Market: symbol, Side: "buy", Quantity: oneBTC, Price: fiftyDollars,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := h.orders.CancelOrder(context.Background(), user, id); err != nil {
		t.Fatal(err)
	}

	// Cancelling answers before the book has acted, so nothing has moved yet.
	if _, locked := h.balance(t, user, "USD"); locked != fiftyDollars {
		t.Errorf("USD locked = %d immediately after cancelling — it should still be held", locked)
	}

	h.drain(t)
	h.settle(t)

	if available, locked := h.balance(t, user, "USD"); available != 1_000_000 || locked != 0 {
		t.Errorf("USD = (%d, %d), want the whole lock back", available, locked)
	}
	if _, status, locked := h.order(t, id); status != models.OrderCancelled || locked != 0 {
		t.Errorf("order = (%q, locked %d), want cancelled and unlocked", status, locked)
	}
	if _, asks := h.orders.Orderbooks[symbol].Depth(0); len(asks) != 0 {
		t.Errorf("asks = %+v, want the cancelled order gone from the book", asks)
	}
}

func TestCancelOrderRefusesWhatIsNotYours(t *testing.T) {
	h := newHarness(t)
	owner := h.user(t, "owner@test", 1_000_000, 0)
	stranger := h.user(t, "stranger@test", 1_000_000, 0)

	id, _ := h.orders.CreateOrder(context.Background(), owner, OrderReq{
		Market: symbol, Side: "buy", Quantity: oneBTC, Price: fiftyDollars,
	})

	// Indistinguishable from an order that does not exist, on purpose: telling
	// them apart would let anyone map which ids are real.
	if err := h.orders.CancelOrder(context.Background(), stranger, id); !errors.Is(err, stores.ErrOrderNotFound) {
		t.Fatalf("cancelling another user's order = %v, want ErrOrderNotFound", err)
	}
	if err := h.orders.CancelOrder(context.Background(), owner, 999_999); !errors.Is(err, stores.ErrOrderNotFound) {
		t.Fatalf("cancelling a missing order = %v, want ErrOrderNotFound", err)
	}
	if _, locked := h.balance(t, owner, "USD"); locked != fiftyDollars {
		t.Error("a refused cancellation released funds")
	}
}

func TestCancelOrderRefusesATerminalOrder(t *testing.T) {
	h := newHarness(t)
	buyer := h.user(t, "buyer@test", 1_000_000, 0)
	seller := h.user(t, "seller@test", 0, oneBTC*5)

	h.orders.CreateOrder(context.Background(), seller, OrderReq{
		Market: symbol, Side: "sell", Quantity: oneBTC, Price: fiftyDollars,
	})
	buyID, _ := h.orders.CreateOrder(context.Background(), buyer, OrderReq{
		Market: symbol, Side: "buy", Quantity: oneBTC, Price: fiftyDollars,
	})
	h.drain(t)
	h.settle(t)

	err := h.orders.CancelOrder(context.Background(), buyer, buyID)
	if !errors.Is(err, ErrOrderNotCancellable) {
		t.Fatalf("cancelling a filled order = %v, want ErrOrderNotCancellable", err)
	}
}

// ── recovery ──────────────────────────────────────────────────────────────

func TestCancelRestingOrdersUnwindsEveryMarket(t *testing.T) {
	h := newHarness(t)
	buyer := h.user(t, "buyer@test", 1_000_000, 0)
	seller := h.user(t, "seller@test", 0, oneBTC*5)

	before := h.totals(t)

	h.orders.CreateOrder(context.Background(), buyer, OrderReq{
		Market: symbol, Side: "buy", Quantity: oneBTC, Price: fiftyDollars,
	})
	h.orders.CreateOrder(context.Background(), seller, OrderReq{
		Market: symbol, Side: "sell", Quantity: oneBTC, Price: fiftyDollars * 3,
	})

	if err := h.orders.CancelRestingOrders(context.Background()); err != nil {
		t.Fatal(err)
	}

	// Every lock belongs to some live order, so cancelling them all must leave
	// nothing held anywhere.
	for _, pair := range [][2]interface{}{{buyer, "USD"}, {seller, "BTC"}} {
		user, currency := pair[0].(int64), pair[1].(string)
		if _, locked := h.balance(t, user, currency); locked != 0 {
			t.Errorf("user %d still holds %d locked %s after the flush", user, locked, currency)
		}
	}
	for currency, total := range before {
		if h.totals(t)[currency] != total {
			t.Errorf("%s total moved during a flush", currency)
		}
	}

	// Running it again finds nothing to do.
	if err := h.orders.CancelRestingOrders(context.Background()); err != nil {
		t.Fatalf("second flush: %v", err)
	}
}

func TestReplayPendingAppliesWhatWasOwed(t *testing.T) {
	h := newHarness(t)
	buyer := h.user(t, "buyer@test", 1_000_000, 0)
	seller := h.user(t, "seller@test", 0, oneBTC*5)

	buyID, _ := h.orders.CreateOrder(context.Background(), buyer, OrderReq{
		Market: symbol, Side: "buy", Quantity: oneBTC, Price: fiftyDollars,
	})
	sellID, _ := h.orders.CreateOrder(context.Background(), seller, OrderReq{
		Market: symbol, Side: "sell", Quantity: oneBTC, Price: fiftyDollars,
	})

	// A fill the previous run's engine produced and never applied.
	fill := models.Trade{
		Market: symbol, RestingOrderID: sellID, IncomingOrderID: buyID,
		IncomingSide: "buy", Quantity: oneBTC / 2, Price: fiftyDollars,
		ExecutionTime: time.Now().UTC(),
	}
	if _, err := h.outbox.Append(context.Background(), models.LedgerEvent{Fill: &fill}); err != nil {
		t.Fatal(err)
	}

	if err := h.trades.ReplayPending(context.Background()); err != nil {
		t.Fatal(err)
	}

	if filled, status, _ := h.order(t, buyID); filled != oneBTC/2 || status != models.OrderPartiallyFilled {
		t.Errorf("buy order = (%d, %q), want half filled", filled, status)
	}

	pending, err := h.outbox.Pending(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 0 {
		t.Errorf("%d event(s) still owed after a replay", len(pending))
	}
}

// A poison event must not wedge recovery: it is written off, loudly, and the
// rest of the queue still gets applied.
func TestReplayPendingWritesOffWhatItCannotApply(t *testing.T) {
	h := newHarness(t)
	buyer := h.user(t, "buyer@test", 1_000_000, 0)
	seller := h.user(t, "seller@test", 0, oneBTC*5)

	buyID, _ := h.orders.CreateOrder(context.Background(), buyer, OrderReq{
		Market: symbol, Side: "buy", Quantity: oneBTC, Price: fiftyDollars,
	})
	sellID, _ := h.orders.CreateOrder(context.Background(), seller, OrderReq{
		Market: symbol, Side: "sell", Quantity: oneBTC, Price: fiftyDollars,
	})

	// Orders that do not exist, followed by a fill that does.
	poison := models.Trade{
		Market: symbol, RestingOrderID: 999_998, IncomingOrderID: 999_999,
		IncomingSide: "buy", Quantity: 1_000, Price: fiftyDollars,
		ExecutionTime: time.Now().UTC(),
	}
	good := models.Trade{
		Market: symbol, RestingOrderID: sellID, IncomingOrderID: buyID,
		IncomingSide: "buy", Quantity: oneBTC / 2, Price: fiftyDollars,
		ExecutionTime: time.Now().UTC(),
	}
	h.outbox.Append(context.Background(), models.LedgerEvent{Fill: &poison})
	h.outbox.Append(context.Background(), models.LedgerEvent{Fill: &good})

	if err := h.trades.ReplayPending(context.Background()); err != nil {
		t.Fatalf("recovery gave up because one event could not be applied: %v", err)
	}

	failed, err := h.outbox.FailedCount(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if failed != 1 {
		t.Errorf("FailedCount = %d, want the one poison event", failed)
	}
	if filled, _, _ := h.order(t, buyID); filled != oneBTC/2 {
		t.Errorf("buy order filled = %d — the good event behind the poison one was skipped", filled)
	}
}
