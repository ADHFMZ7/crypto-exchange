package stores

import (
	"context"
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
	newTestStore(t)
	outbox := &OutboxStore{testPool}
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

	if err := outbox.MarkApplied(ctx, id); err != nil {
		t.Fatal(err)
	}
	if pending, _ := outbox.Pending(ctx, 50); len(pending) != 0 {
		t.Fatalf("still owed after being applied: %+v", pending)
	}
}

// Order is the contract: a cancellation that overtook its own fill would zero a
// lock the fill still needs.
func TestOutboxReplaysInTheOrderEventsWereProduced(t *testing.T) {
	newTestStore(t)
	outbox := &OutboxStore{testPool}
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
	newTestStore(t)
	outbox := &OutboxStore{testPool}
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
	newTestStore(t)
	outbox := &OutboxStore{testPool}
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
	newTestStore(t)
	outbox := &OutboxStore{testPool}
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
