package stores

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ADHFMZ7/crypto-exchange/internal/models"
)

// One whole BTC in satoshis, and a price of $50 per whole BTC in cents. The
// notional is then 100_000_000 * 5_000 / 10^8 = 5_000 cents, which is small
// enough to check by hand and the exact shape that produced the bug below.
const (
	oneBTC       = int64(100_000_000)
	fiftyDollars = int64(5_000)
)

func trade(restingID, incomingID int64, side string, quantity, price int64) models.Trade {
	return models.Trade{
		Market:          "BTC-USD",
		RestingOrderID:  restingID,
		IncomingOrderID: incomingID,
		IncomingSide:    side,
		Quantity:        quantity,
		Price:           price,
		ExecutionTime:   time.Now(),
	}
}

// The regression test.
//
// moveBalance was written as a single INSERT ... ON CONFLICT DO UPDATE. A CHECK
// constraint is evaluated against the proposed insert row, before ON CONFLICT
// arbitration, so the buyer's negative locked delta tripped
// balances_non_negative even though the row existed and the update would have
// landed on exactly zero. Every settlement failed and rolled back.
func TestSettleReleasesBuyerLockWhenBalanceRowAlreadyExists(t *testing.T) {
	store := newTestStore(t)

	buyer := seedUser(t, "buyer@test")
	seller := seedUser(t, "seller@test")

	// The buyer already holds a USD row with funds locked — the case the upsert
	// mishandled. The seller already holds the BTC they are selling.
	seedBalance(t, buyer, "USD", 995_000, fiftyDollars)
	seedBalance(t, seller, "BTC", 900_000_000, oneBTC)

	buyOrder := seedOrder(t, buyer, "buy", oneBTC, fiftyDollars, fiftyDollars)
	sellOrder := seedOrder(t, seller, "sell", oneBTC, fiftyDollars, oneBTC)

	err := store.Settle(context.Background(),
		trade(sellOrder, buyOrder, "buy", oneBTC, fiftyDollars),
		"BTC", "USD", fiftyDollars)
	if err != nil {
		t.Fatalf("Settle returned %v, want nil", err)
	}

	if available, locked := readBalance(t, buyer, "USD"); locked != 0 || available != 995_000 {
		t.Fatalf("buyer USD = (%d available, %d locked), want (995000, 0)", available, locked)
	}
}

func TestSettleFullFillMovesBothSides(t *testing.T) {
	store := newTestStore(t)

	buyer := seedUser(t, "buyer@test")
	seller := seedUser(t, "seller@test")
	seedBalance(t, buyer, "USD", 0, fiftyDollars)
	seedBalance(t, seller, "BTC", 0, oneBTC)

	buyOrder := seedOrder(t, buyer, "buy", oneBTC, fiftyDollars, fiftyDollars)
	sellOrder := seedOrder(t, seller, "sell", oneBTC, fiftyDollars, oneBTC)

	if err := store.Settle(context.Background(),
		trade(sellOrder, buyOrder, "buy", oneBTC, fiftyDollars),
		"BTC", "USD", fiftyDollars); err != nil {
		t.Fatal(err)
	}

	// The buyer gave up quote and received base.
	if available, locked := readBalance(t, buyer, "USD"); available != 0 || locked != 0 {
		t.Fatalf("buyer USD = (%d, %d), want (0, 0)", available, locked)
	}
	if available, locked := readBalance(t, buyer, "BTC"); available != oneBTC || locked != 0 {
		t.Fatalf("buyer BTC = (%d, %d), want (%d, 0)", available, locked, oneBTC)
	}

	// The seller the reverse.
	if available, locked := readBalance(t, seller, "BTC"); available != 0 || locked != 0 {
		t.Fatalf("seller BTC = (%d, %d), want (0, 0)", available, locked)
	}
	if available, locked := readBalance(t, seller, "USD"); available != fiftyDollars || locked != 0 {
		t.Fatalf("seller USD = (%d, %d), want (%d, 0)", available, locked, fiftyDollars)
	}

	for _, id := range []int64{buyOrder, sellOrder} {
		filled, status, lockedRemaining := readOrder(t, id)
		if filled != oneBTC {
			t.Fatalf("order %d filled = %d, want %d", id, filled, oneBTC)
		}
		if status != models.OrderFilled {
			t.Fatalf("order %d status = %q, want %q", id, status, models.OrderFilled)
		}
		if lockedRemaining != 0 {
			t.Fatalf("order %d locked_remaining = %d, want 0", id, lockedRemaining)
		}
	}
}

// A buyer receiving a currency they have never held has no row to update, so
// the insert path has to work too — it is the half of moveBalance that was
// never broken and must not regress while fixing the other half.
func TestSettleCreatesBalanceRowForACurrencyNeverHeld(t *testing.T) {
	store := newTestStore(t)

	buyer := seedUser(t, "buyer@test")
	seller := seedUser(t, "seller@test")
	seedBalance(t, buyer, "USD", 0, fiftyDollars) // no BTC row at all
	seedBalance(t, seller, "BTC", 0, oneBTC)      // no USD row at all

	buyOrder := seedOrder(t, buyer, "buy", oneBTC, fiftyDollars, fiftyDollars)
	sellOrder := seedOrder(t, seller, "sell", oneBTC, fiftyDollars, oneBTC)

	if err := store.Settle(context.Background(),
		trade(sellOrder, buyOrder, "buy", oneBTC, fiftyDollars),
		"BTC", "USD", fiftyDollars); err != nil {
		t.Fatal(err)
	}

	if available, _ := readBalance(t, buyer, "BTC"); available != oneBTC {
		t.Fatalf("buyer BTC available = %d, want %d", available, oneBTC)
	}
	if available, _ := readBalance(t, seller, "USD"); available != fiftyDollars {
		t.Fatalf("seller USD available = %d, want %d", available, fiftyDollars)
	}
}

// Half the order fills, so both sides stay open with part of their lock intact.
func TestSettlePartialFillLeavesOrdersOpen(t *testing.T) {
	store := newTestStore(t)

	buyer := seedUser(t, "buyer@test")
	seller := seedUser(t, "seller@test")
	seedBalance(t, buyer, "USD", 0, fiftyDollars)
	seedBalance(t, seller, "BTC", 0, oneBTC)

	buyOrder := seedOrder(t, buyer, "buy", oneBTC, fiftyDollars, fiftyDollars)
	sellOrder := seedOrder(t, seller, "sell", oneBTC, fiftyDollars, oneBTC)

	half := oneBTC / 2
	halfCost := fiftyDollars / 2

	if err := store.Settle(context.Background(),
		trade(sellOrder, buyOrder, "buy", half, fiftyDollars),
		"BTC", "USD", halfCost); err != nil {
		t.Fatal(err)
	}

	filled, status, lockedRemaining := readOrder(t, buyOrder)
	if filled != half {
		t.Fatalf("buy order filled = %d, want %d", filled, half)
	}
	if status != models.OrderPartiallyFilled {
		t.Fatalf("buy order status = %q, want %q", status, models.OrderPartiallyFilled)
	}
	if lockedRemaining != fiftyDollars-halfCost {
		t.Fatalf("buy order locked_remaining = %d, want %d", lockedRemaining, fiftyDollars-halfCost)
	}

	if _, locked := readBalance(t, buyer, "USD"); locked != fiftyDollars-halfCost {
		t.Fatalf("buyer USD locked = %d, want %d", locked, fiftyDollars-halfCost)
	}
}

// A buy that crosses a cheaper resting sell executes at the seller's price. The
// difference was locked at the buyer's limit and has to come back to them, or
// it stays locked forever and the books never reconcile.
func TestSettleRefundsPriceImprovementOnTheFinalFill(t *testing.T) {
	store := newTestStore(t)

	buyer := seedUser(t, "buyer@test")
	seller := seedUser(t, "seller@test")

	// Buyer was willing to pay $60, locked accordingly. The resting sell is $50.
	buyerLimit := int64(6_000)
	executionPrice := fiftyDollars

	seedBalance(t, buyer, "USD", 0, buyerLimit)
	seedBalance(t, seller, "BTC", 0, oneBTC)

	buyOrder := seedOrder(t, buyer, "buy", oneBTC, buyerLimit, buyerLimit)
	sellOrder := seedOrder(t, seller, "sell", oneBTC, executionPrice, oneBTC)

	if err := store.Settle(context.Background(),
		trade(sellOrder, buyOrder, "buy", oneBTC, executionPrice),
		"BTC", "USD", executionPrice); err != nil {
		t.Fatal(err)
	}

	refund := buyerLimit - executionPrice
	available, locked := readBalance(t, buyer, "USD")
	if available != refund {
		t.Fatalf("buyer USD available = %d, want %d refunded", available, refund)
	}
	if locked != 0 {
		t.Fatalf("buyer USD locked = %d, want 0 — the whole lock is resolved", locked)
	}
	if _, seller_locked := readBalance(t, seller, "BTC"); seller_locked != 0 {
		t.Fatalf("seller BTC locked = %d, want 0", seller_locked)
	}
	// The seller is paid the execution price, not the buyer's limit.
	if available, _ := readBalance(t, seller, "USD"); available != executionPrice {
		t.Fatalf("seller USD available = %d, want %d", available, executionPrice)
	}
}

// Settlement moves value between accounts. It must never create or destroy it.
func TestSettleConservesValue(t *testing.T) {
	store := newTestStore(t)

	buyer := seedUser(t, "buyer@test")
	seller := seedUser(t, "seller@test")
	seedBalance(t, buyer, "USD", 12_345, fiftyDollars)
	seedBalance(t, seller, "BTC", 42, oneBTC)

	before := totalPerCurrency(t)

	buyOrder := seedOrder(t, buyer, "buy", oneBTC, fiftyDollars, fiftyDollars)
	sellOrder := seedOrder(t, seller, "sell", oneBTC, fiftyDollars, oneBTC)

	if err := store.Settle(context.Background(),
		trade(sellOrder, buyOrder, "buy", oneBTC, fiftyDollars),
		"BTC", "USD", fiftyDollars); err != nil {
		t.Fatal(err)
	}

	after := totalPerCurrency(t)
	for currency, total := range before {
		if after[currency] != total {
			t.Fatalf("%s total = %d, want %d — settlement created or destroyed value",
				currency, after[currency], total)
		}
	}
}

// A failure must leave nothing behind: no trade row, no advanced fill, no moved
// funds. Overfilling trips orders_filled_within_quantity part-way through.
func TestSettleRollsBackEverythingOnFailure(t *testing.T) {
	store := newTestStore(t)

	buyer := seedUser(t, "buyer@test")
	seller := seedUser(t, "seller@test")
	seedBalance(t, buyer, "USD", 0, fiftyDollars)
	seedBalance(t, seller, "BTC", 0, oneBTC)

	buyOrder := seedOrder(t, buyer, "buy", oneBTC, fiftyDollars, fiftyDollars)
	sellOrder := seedOrder(t, seller, "sell", oneBTC, fiftyDollars, oneBTC)

	// Twice the order quantity.
	err := store.Settle(context.Background(),
		trade(sellOrder, buyOrder, "buy", oneBTC*2, fiftyDollars),
		"BTC", "USD", fiftyDollars*2)
	if err == nil {
		t.Fatal("Settle accepted a fill larger than the order")
	}

	var trades int
	if err := testPool.QueryRow(context.Background(), `SELECT count(*) FROM trades`).Scan(&trades); err != nil {
		t.Fatal(err)
	}
	if trades != 0 {
		t.Fatalf("%d trade rows survived a failed settlement", trades)
	}

	if filled, status, _ := readOrder(t, buyOrder); filled != 0 || status != "open" {
		t.Fatalf("buy order advanced to (%d, %q) despite failure", filled, status)
	}
	if available, locked := readBalance(t, buyer, "USD"); available != 0 || locked != fiftyDollars {
		t.Fatalf("buyer USD = (%d, %d), want untouched (0, %d)", available, locked, fiftyDollars)
	}
}

// Debiting a balance row that does not exist means settlement is working from
// state the ledger never saw. It must fail rather than create a negative row.
func TestSettleRejectsDebitOfAMissingBalance(t *testing.T) {
	store := newTestStore(t)

	buyer := seedUser(t, "buyer@test")
	seller := seedUser(t, "seller@test")
	seedBalance(t, seller, "BTC", 0, oneBTC) // buyer has no USD row at all

	buyOrder := seedOrder(t, buyer, "buy", oneBTC, fiftyDollars, fiftyDollars)
	sellOrder := seedOrder(t, seller, "sell", oneBTC, fiftyDollars, oneBTC)

	err := store.Settle(context.Background(),
		trade(sellOrder, buyOrder, "buy", oneBTC, fiftyDollars),
		"BTC", "USD", fiftyDollars)
	if !errors.Is(err, ErrNoBalance) {
		t.Fatalf("Settle err = %v, want ErrNoBalance", err)
	}
}

func totalPerCurrency(t *testing.T) map[string]int64 {
	t.Helper()

	rows, err := testPool.Query(context.Background(),
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
