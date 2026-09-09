package stores

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ADHFMZ7/crypto-exchange/internal/models"
)

/*
The ledger outbox.

Its job is that an effect the matching engine produced can be delayed but not
lost. So the tests are about what survives: a recorded event stays owed until it
is explicitly applied or explicitly written off, and it comes back in the order
it was produced.
*/

func fillEvent(resting, incoming int64) models.LedgerEvent {
	return models.LedgerEvent{Fill: &models.Trade{
		Market:          "BTC-USD",
		RestingOrderID:  resting,
		IncomingOrderID: incoming,
		IncomingSide:    "buy",
		Quantity:        oneBTC,
		Price:           fiftyDollars,
		ExecutionTime:   time.Now().UTC().Truncate(time.Millisecond),
	}}
}

func cancelEvent(orderID int64) models.LedgerEvent {
	return models.LedgerEvent{Cancel: &models.OrderCancel{
		Market: "BTC-USD", OrderID: orderID, Side: "sell",
	}}
}

func TestOutboxKeepsAnEventOwedUntilItIsResolved(t *testing.T) {
	_, _, outbox, _ := newTestStores(t)
	ctx := context.Background()

	id, err := outbox.Append(ctx, fillEvent(1, 2))
	if err != nil {
		t.Fatal(err)
	}

	pending, err := outbox.Pending(ctx, 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 1 || pending[0].ID != id {
		t.Fatalf("pending = %+v, want the event just recorded", pending)
	}

	// Claiming is how an effect marks its own event, in its own transaction.
	tx, err := testPool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := claimEvent(ctx, tx, id); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	if pending, _ := outbox.Pending(ctx, 50); len(pending) != 0 {
		t.Fatalf("still owed after being applied: %+v", pending)
	}
}

// The claim is what makes replay exactly-once: the second attempt is told the
// event is spoken for, and its transaction rolls back rather than applying the
// effect again.
func TestClaimingAnEventTwiceIsRefused(t *testing.T) {
	_, _, outbox, _ := newTestStores(t)
	ctx := context.Background()

	id, err := outbox.Append(ctx, fillEvent(1, 2))
	if err != nil {
		t.Fatal(err)
	}

	first, _ := testPool.Begin(ctx)
	if err := claimEvent(ctx, first, id); err != nil {
		t.Fatalf("first claim: %v", err)
	}
	if err := first.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	second, _ := testPool.Begin(ctx)
	defer second.Rollback(ctx)
	if err := claimEvent(ctx, second, id); !errors.Is(err, ErrAlreadyApplied) {
		t.Fatalf("second claim = %v, want ErrAlreadyApplied", err)
	}
}

// A written-off event must not be resurrected by a late replay.
func TestClaimingAFailedEventIsRefused(t *testing.T) {
	_, _, outbox, _ := newTestStores(t)
	ctx := context.Background()

	id, err := outbox.Append(ctx, fillEvent(3, 4))
	if err != nil {
		t.Fatal(err)
	}
	if err := outbox.MarkFailed(ctx, id, "gave up"); err != nil {
		t.Fatal(err)
	}

	tx, _ := testPool.Begin(ctx)
	defer tx.Rollback(ctx)
	if err := claimEvent(ctx, tx, id); !errors.Is(err, ErrAlreadyApplied) {
		t.Fatalf("claim of a failed event = %v, want ErrAlreadyApplied", err)
	}
}

// NoEvent means "nothing is driving this" — the startup flush, and tests.
func TestClaimingNoEventIsANoOp(t *testing.T) {
	newTestStore(t)
	ctx := context.Background()

	tx, _ := testPool.Begin(ctx)
	defer tx.Rollback(ctx)
	if err := claimEvent(ctx, tx, NoEvent); err != nil {
		t.Fatalf("claiming NoEvent = %v, want nil", err)
	}
}

// Order is the contract: a cancellation that overtook its own fill would zero a
// lock the fill still needs.
func TestOutboxReplaysInTheOrderEventsWereProduced(t *testing.T) {
	_, _, outbox, _ := newTestStores(t)
	ctx := context.Background()

	var ids []int64
	for i := int64(1); i <= 4; i++ {
		id, err := outbox.Append(ctx, fillEvent(i, i+100))
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	if _, err := outbox.Append(ctx, cancelEvent(1)); err != nil {
		t.Fatal(err)
	}

	pending, err := outbox.Pending(ctx, 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 5 {
		t.Fatalf("got %d pending, want 5", len(pending))
	}
	for i, id := range ids {
		if pending[i].ID != id {
			t.Fatalf("pending[%d] = %d, want %d — replay order does not match production order",
				i, pending[i].ID, id)
		}
	}
	if pending[4].Kind != models.LedgerCancel {
		t.Errorf("last event kind = %q, want the cancellation last", pending[4].Kind)
	}
}

// A written-off event stops being owed but must not stop being visible — it is
// the only record that the book and the ledger disagree about something.
func TestOutboxLeavesFailedEventsWhereTheyCanBeFound(t *testing.T) {
	_, _, outbox, _ := newTestStores(t)
	ctx := context.Background()

	id, err := outbox.Append(ctx, fillEvent(7, 8))
	if err != nil {
		t.Fatal(err)
	}
	if err := outbox.MarkFailed(ctx, id, "settle: something went wrong"); err != nil {
		t.Fatal(err)
	}

	if pending, _ := outbox.Pending(ctx, 50); len(pending) != 0 {
		t.Fatalf("a written-off event is still being retried: %+v", pending)
	}

	failed, err := outbox.FailedCount(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if failed != 1 {
		t.Fatalf("FailedCount = %d, want 1", failed)
	}

	var attempts int
	var lastError string
	if err := testPool.QueryRow(ctx,
		`SELECT attempts, last_error FROM ledger_events WHERE id = $1`, id).
		Scan(&attempts, &lastError); err != nil {
		t.Fatal(err)
	}
	if attempts != 1 || lastError == "" {
		t.Fatalf("attempts=%d last_error=%q — the reason must survive", attempts, lastError)
	}
}

func TestOutboxRecordsAttemptsWithoutGivingUp(t *testing.T) {
	_, _, outbox, _ := newTestStores(t)
	ctx := context.Background()

	id, err := outbox.Append(ctx, fillEvent(1, 2))
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if err := outbox.RecordAttempt(ctx, id, "connection refused"); err != nil {
			t.Fatal(err)
		}
	}

	if pending, _ := outbox.Pending(ctx, 50); len(pending) != 1 {
		t.Fatal("a failed attempt must leave the event owed")
	}

	var attempts int
	if err := testPool.QueryRow(ctx, `SELECT attempts FROM ledger_events WHERE id = $1`, id).
		Scan(&attempts); err != nil {
		t.Fatal(err)
	}
	if attempts != 3 {
		t.Fatalf("attempts = %d, want 3", attempts)
	}
}

// What is stored has to come back as what went in, or replay applies something
// other than what the engine decided.
func TestOutboxRoundTripsBothKinds(t *testing.T) {
	_, _, outbox, _ := newTestStores(t)
	ctx := context.Background()

	original := fillEvent(11, 22)
	if _, err := outbox.Append(ctx, original); err != nil {
		t.Fatal(err)
	}
	if _, err := outbox.Append(ctx, cancelEvent(33)); err != nil {
		t.Fatal(err)
	}

	pending, err := outbox.Pending(ctx, 50)
	if err != nil {
		t.Fatal(err)
	}

	back, err := DecodeEvent(pending[0].Kind, pending[0].Payload)
	if err != nil {
		t.Fatal(err)
	}
	if back.Fill == nil {
		t.Fatal("first event did not come back as a fill")
	}
	if *back.Fill != *original.Fill {
		t.Fatalf("fill round-tripped as %+v, want %+v", *back.Fill, *original.Fill)
	}

	back, err = DecodeEvent(pending[1].Kind, pending[1].Payload)
	if err != nil {
		t.Fatal(err)
	}
	if back.Cancel == nil || back.Cancel.OrderID != 33 || back.Cancel.Side != "sell" {
		t.Fatalf("cancellation round-tripped as %+v", back.Cancel)
	}
}

// The regression this claim exists for.
//
// Marking an event applied in a separate statement after the effect committed
// left a window: if the mark failed, the row stayed owed, the next boot
// replayed it, and the fill landed twice — a second trade row, filled_quantity
// advanced again, the money moved again, and no error to show for it. Orders
// large enough not to trip orders_filled_within_quantity is exactly the shape
// that slipped through, so that is the shape tested here.
func TestReplayingAFillAppliesItExactlyOnce(t *testing.T) {
	store, _, outbox, _ := newTestStores(t)
	ctx := context.Background()

	buyer := seedUser(t, "buyer@test")
	seller := seedUser(t, "seller@test")
	seedBalance(t, buyer, "USD", 0, fiftyDollars*10)
	seedBalance(t, seller, "BTC", 0, oneBTC*10)
	buyOrder := seedOrder(t, buyer, "buy", oneBTC*10, fiftyDollars, fiftyDollars*10)
	sellOrder := seedOrder(t, seller, "sell", oneBTC*10, fiftyDollars, oneBTC*10)

	fill := trade(sellOrder, buyOrder, "buy", oneBTC, fiftyDollars)
	id, err := outbox.Append(ctx, models.LedgerEvent{Fill: &fill})
	if err != nil {
		t.Fatal(err)
	}

	if err := store.Settle(ctx, id, fill, "BTC", "USD", fiftyDollars); err != nil {
		t.Fatal(err)
	}

	sellerUSD, _ := readBalance(t, seller, "USD")
	buyerBTC, _ := readBalance(t, buyer, "BTC")
	filled, _, lockedRemaining := readOrder(t, buyOrder)

	// The replay a restart would perform.
	err = store.Settle(ctx, id, fill, "BTC", "USD", fiftyDollars)
	if !errors.Is(err, ErrAlreadyApplied) {
		t.Fatalf("second Settle = %v, want ErrAlreadyApplied", err)
	}

	if got, _ := readBalance(t, seller, "USD"); got != sellerUSD {
		t.Errorf("seller USD = %d, want %d — paid twice for one fill", got, sellerUSD)
	}
	if got, _ := readBalance(t, buyer, "BTC"); got != buyerBTC {
		t.Errorf("buyer BTC = %d, want %d — credited twice for one fill", got, buyerBTC)
	}
	if got, _, gotLocked := readOrder(t, buyOrder); got != filled || gotLocked != lockedRemaining {
		t.Errorf("buy order = (filled %d, locked %d), want (%d, %d)",
			got, gotLocked, filled, lockedRemaining)
	}

	var trades int
	if err := testPool.QueryRow(ctx, `SELECT count(*) FROM trades`).Scan(&trades); err != nil {
		t.Fatal(err)
	}
	if trades != 1 {
		t.Errorf("%d trade rows for one fill — the tape would show a trade that never happened", trades)
	}
}

// The same hazard on the other half. Releasing a lock twice would hand back
// money the order no longer held.
func TestReplayingACancellationReleasesExactlyOnce(t *testing.T) {
	_, wallets, outbox, _ := newTestStores(t)
	ctx := context.Background()

	user := seedUser(t, "trader@test")
	seedBalance(t, user, "USD", 0, fiftyDollars)
	order := seedOrder(t, user, "buy", oneBTC, fiftyDollars, fiftyDollars)

	id, err := outbox.Append(ctx, cancelEvent(order))
	if err != nil {
		t.Fatal(err)
	}

	if err := wallets.CancelOrder(ctx, id, order, "USD"); err != nil {
		t.Fatal(err)
	}
	available, locked := readBalance(t, user, "USD")

	err = wallets.CancelOrder(ctx, id, order, "USD")
	if !errors.Is(err, ErrAlreadyApplied) {
		t.Fatalf("second CancelOrder = %v, want ErrAlreadyApplied", err)
	}

	if gotA, gotL := readBalance(t, user, "USD"); gotA != available || gotL != locked {
		t.Errorf("USD = (%d, %d), want (%d, %d) — released twice", gotA, gotL, available, locked)
	}
}
