package stores

import (
	"context"
	"errors"
	"testing"
)

/*
Cancellation returns what an order still had locked.

These live beside the settlement tests because they contend for the same row:
locked_remaining is decremented by fills and zeroed by a cancellation, and the
whole point of the conditional UPDATE in CancelOrder is deciding which of the
two got there first.
*/

func TestCancelOrderReturnsTheUnspentLock(t *testing.T) {
	store := &WalletStore{testPool}
	newTestStore(t) // clears the tables

	user := seedUser(t, "trader@test")
	seedBalance(t, user, "USD", 1_000, fiftyDollars)
	order := seedOrder(t, user, "buy", oneBTC, fiftyDollars, fiftyDollars)

	if err := store.CancelOrder(context.Background(), order, "USD"); err != nil {
		t.Fatal(err)
	}

	if available, locked := readBalance(t, user, "USD"); available != 1_000+fiftyDollars || locked != 0 {
		t.Fatalf("USD = (%d, %d), want (%d, 0)", available, locked, 1_000+fiftyDollars)
	}

	_, status, lockedRemaining := readOrder(t, order)
	if status != "cancelled" || lockedRemaining != 0 {
		t.Fatalf("order = (%q, %d), want cancelled with nothing locked", status, lockedRemaining)
	}
}

// Only the part not already spent on fills comes back.
func TestCancelOrderReturnsOnlyWhatIsLeftAfterPartialFills(t *testing.T) {
	store := &WalletStore{testPool}
	newTestStore(t)

	user := seedUser(t, "trader@test")
	seedBalance(t, user, "USD", 0, 2_000)

	var order int64
	err := testPool.QueryRow(context.Background(), `
		INSERT INTO orders (user_id, quantity, filled_quantity, price_each, side, market, status, locked_remaining)
		VALUES ($1, $2, $3, $4, 'buy', 'BTC-USD', 'partially_filled', $5)
		RETURNING id
	`, user, oneBTC, oneBTC/2, fiftyDollars, 2_000).Scan(&order)
	if err != nil {
		t.Fatal(err)
	}

	if err := store.CancelOrder(context.Background(), order, "USD"); err != nil {
		t.Fatal(err)
	}
	if available, locked := readBalance(t, user, "USD"); available != 2_000 || locked != 0 {
		t.Fatalf("USD = (%d, %d), want (2000, 0)", available, locked)
	}
}

// A lock released twice would drive a live order's locked_remaining negative and
// strand every later fill, so the second attempt has to be refused.
func TestCancelOrderIsRefusedOnceTheOrderIsTerminal(t *testing.T) {
	store := &WalletStore{testPool}
	newTestStore(t)

	user := seedUser(t, "trader@test")
	seedBalance(t, user, "USD", 0, fiftyDollars)
	order := seedOrder(t, user, "buy", oneBTC, fiftyDollars, fiftyDollars)

	if err := store.CancelOrder(context.Background(), order, "USD"); err != nil {
		t.Fatal(err)
	}
	if err := store.CancelOrder(context.Background(), order, "USD"); !errors.Is(err, ErrNotCancellable) {
		t.Fatalf("second cancel err = %v, want ErrNotCancellable", err)
	}

	if available, locked := readBalance(t, user, "USD"); available != fiftyDollars || locked != 0 {
		t.Fatalf("USD = (%d, %d) — the lock was released twice", available, locked)
	}
}

// The fill that beat the cancellation wins: a filled order stays filled and
// nothing is returned, because nothing is left.
func TestCancelOrderLosesToAFillThatSettledFirst(t *testing.T) {
	store := &WalletStore{testPool}
	newTestStore(t)

	user := seedUser(t, "trader@test")
	seedBalance(t, user, "USD", 0, 0)

	var order int64
	err := testPool.QueryRow(context.Background(), `
		INSERT INTO orders (user_id, quantity, filled_quantity, price_each, side, market, status, locked_remaining)
		VALUES ($1, $2, $2, $3, 'buy', 'BTC-USD', 'filled', 0)
		RETURNING id
	`, user, oneBTC, fiftyDollars).Scan(&order)
	if err != nil {
		t.Fatal(err)
	}

	if err := store.CancelOrder(context.Background(), order, "USD"); !errors.Is(err, ErrNotCancellable) {
		t.Fatalf("cancel err = %v, want ErrNotCancellable", err)
	}
	if _, status, _ := readOrder(t, order); status != "filled" {
		t.Fatalf("order status = %q, want it left filled", status)
	}
}
