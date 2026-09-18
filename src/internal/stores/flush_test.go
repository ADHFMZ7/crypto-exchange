package stores

import (
	"context"
	"testing"
)

/*
The boot-time flush.

Its whole justification is that it cannot be wrong, so the tests are about the
post-condition rather than the mechanics: after a flush there is nothing resting
and nothing locked, whatever the book had been doing beforehand.

The invariant worth stating out loud is that every minor unit in balances.locked
belongs to some live order's locked_remaining. Cancelling every live order must
therefore drive locked to zero across every currency — if it does not, a lock
existed that no order accounted for.
*/

func totalLockedPerCurrency(t *testing.T) map[string]int64 {
	t.Helper()

	rows, err := testPool.Query(context.Background(),
		`SELECT currency, sum(locked) FROM balances GROUP BY currency`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()

	locked := map[string]int64{}
	for rows.Next() {
		var currency string
		var total int64
		if err := rows.Scan(&currency, &total); err != nil {
			t.Fatal(err)
		}
		locked[currency] = total
	}
	return locked
}

func countByStatus(t *testing.T, status string) int {
	t.Helper()

	var n int
	if err := testPool.QueryRow(context.Background(),
		`SELECT count(*) FROM orders WHERE status = $1`, status).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// The state a restart actually leaves behind: orders resting on both sides,
// one of them half filled, all of them holding locks against a book that no
// longer exists.
func seedRestartState(t *testing.T) (buyer, seller int64, orders []OrderRelease) {
	t.Helper()

	buyer = seedUser(t, "buyer@test")
	seller = seedUser(t, "seller@test")

	seedBalance(t, buyer, "USD", 500_000, fiftyDollars*3)
	seedBalance(t, seller, "BTC", 900_000_000, oneBTC*2)

	open := seedOrder(t, buyer, "buy", oneBTC, fiftyDollars, fiftyDollars)
	sell := seedOrder(t, seller, "sell", oneBTC, fiftyDollars, oneBTC)

	var partial int64
	err := testPool.QueryRow(context.Background(), `
		INSERT INTO orders (user_id, quantity, filled_quantity, price_each, side, market, status, locked_remaining)
		VALUES ($1, $2, $3, $4, 'buy', 'BTC-USD', 'partially_filled', $5)
		RETURNING id
	`, buyer, oneBTC, oneBTC/2, fiftyDollars, fiftyDollars*2).Scan(&partial)
	if err != nil {
		t.Fatal(err)
	}

	// A second sell that is already locked one-for-one with its quantity.
	seedOrderLock := seedOrder(t, seller, "sell", oneBTC, fiftyDollars, oneBTC)

	return buyer, seller, []OrderRelease{
		{OrderID: open, Currency: "USD"},
		{OrderID: partial, Currency: "USD"},
		{OrderID: sell, Currency: "BTC"},
		{OrderID: seedOrderLock, Currency: "BTC"},
	}
}

func TestCancelRestingOrdersLeavesNothingLocked(t *testing.T) {
	_, store, _, _ := newTestStores(t)

	_, _, releases := seedRestartState(t)

	before := totalPerCurrency(t)

	freed, err := store.CancelRestingOrders(context.Background(), releases)
	if err != nil {
		t.Fatal(err)
	}

	// Every lock belonged to one of those orders, so none may survive.
	for currency, locked := range totalLockedPerCurrency(t) {
		if locked != 0 {
			t.Errorf("%s still has %d locked after the flush", currency, locked)
		}
	}

	if countByStatus(t, "open") != 0 || countByStatus(t, "partially_filled") != 0 {
		t.Errorf("orders still resting: %d open, %d partial",
			countByStatus(t, "open"), countByStatus(t, "partially_filled"))
	}

	// Releasing is a move, not a mint.
	after := totalPerCurrency(t)
	for currency, total := range before {
		if after[currency] != total {
			t.Errorf("%s total = %d, want %d — the flush created or destroyed value",
				currency, after[currency], total)
		}
	}

	if freed["USD"] != fiftyDollars*3 || freed["BTC"] != oneBTC*2 {
		t.Errorf("freed = %v, want 3 x fifty USD and 2 BTC", freed)
	}
}

// Nothing about a terminal order changes: its lock was already resolved, and
// releasing it again would be inventing money.
func TestCancelRestingOrdersLeavesTerminalOrdersAlone(t *testing.T) {
	_, store, _, _ := newTestStores(t)

	user := seedUser(t, "trader@test")
	seedBalance(t, user, "USD", fiftyDollars, 0)

	var filled int64
	err := testPool.QueryRow(context.Background(), `
		INSERT INTO orders (user_id, quantity, filled_quantity, price_each, side, market, status, locked_remaining)
		VALUES ($1, $2, $2, $3, 'buy', 'BTC-USD', 'filled', 0)
		RETURNING id
	`, user, oneBTC, fiftyDollars).Scan(&filled)
	if err != nil {
		t.Fatal(err)
	}

	freed, err := store.CancelRestingOrders(context.Background(),
		[]OrderRelease{{OrderID: filled, Currency: "USD"}})
	if err != nil {
		t.Fatalf("a terminal order should be skipped, not fail the batch: %v", err)
	}
	if len(freed) != 0 {
		t.Errorf("freed = %v, want nothing", freed)
	}
	if _, status, _ := readOrder(t, filled); status != "filled" {
		t.Errorf("status = %q, want it left filled", status)
	}
	if available, locked := readBalance(t, user, "USD"); available != fiftyDollars || locked != 0 {
		t.Errorf("USD = (%d, %d), want (%d, 0) — untouched", available, locked, fiftyDollars)
	}
}

// A crash during startup must not make the flush unsafe to run again.
func TestCancelRestingOrdersIsIdempotent(t *testing.T) {
	_, store, _, _ := newTestStores(t)

	_, _, releases := seedRestartState(t)

	if _, err := store.CancelRestingOrders(context.Background(), releases); err != nil {
		t.Fatal(err)
	}
	after := totalPerCurrency(t)

	freed, err := store.CancelRestingOrders(context.Background(), releases)
	if err != nil {
		t.Fatalf("second flush failed: %v", err)
	}
	if len(freed) != 0 {
		t.Errorf("second flush freed %v, want nothing left to free", freed)
	}

	for currency, total := range after {
		if totalPerCurrency(t)[currency] != total {
			t.Errorf("%s moved on the second flush", currency)
		}
	}
}

func TestCancelRestingOrdersOnAnEmptyExchange(t *testing.T) {
	_, store, _, _ := newTestStores(t)

	freed, err := store.CancelRestingOrders(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(freed) != 0 {
		t.Errorf("freed = %v on an empty exchange", freed)
	}
}
