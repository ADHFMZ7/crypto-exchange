package services

import (
	"errors"
	"fmt"
	"testing"

	"github.com/ADHFMZ7/crypto-exchange/internal/market"
	"github.com/ADHFMZ7/crypto-exchange/internal/models"
	"github.com/ADHFMZ7/crypto-exchange/internal/stores"
	"github.com/jackc/pgx/v5/pgconn"
)

/*
Which settlement failures are worth retrying.

Getting this wrong is asymmetric. Calling a transient failure permanent writes
off a fill that would have applied a moment later, and the book and the ledger
disagree for good. Calling a permanent failure transient costs a few hundred
milliseconds of pointless retries before it is written off anyway — so when in
doubt this leans towards retrying.
*/

func TestPermanentFailuresAreNotRetried(t *testing.T) {
	cases := []struct {
		name string
		err  error
	}{
		{"unknown market", fmt.Errorf("%w: %q", ErrUnknownMarket, "NOPE-USD")},
		{"empty event", models.ErrEmptyLedgerEvent},
		{"no balance to debit", fmt.Errorf("settle: %w", stores.ErrNoBalance)},
		{"non-positive amounts", fmt.Errorf("notional: %w", market.ErrNotPositive)},
		{"notional overflow", fmt.Errorf("notional: %w", market.ErrOverflow)},
		{"invalid side", market.ErrInvalidSide},

		// A fill that would overshoot its order, or a lock that would go
		// negative. The data is wrong, not the moment.
		{"check violation", &pgconn.PgError{Code: "23514", Message: "orders_filled_within_quantity"}},
		{"not null violation", &pgconn.PgError{Code: "23502"}},
		{"foreign key violation", &pgconn.PgError{Code: "23503"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if !permanent(tc.err) {
				t.Errorf("permanent(%v) = false, want true — this would be retried pointlessly", tc.err)
			}
		})
	}
}

func TestTransientFailuresAreRetried(t *testing.T) {
	cases := []struct {
		name string
		err  error
	}{
		{"connection refused", errors.New("dial tcp: connection refused")},
		{"lock timeout", &pgconn.PgError{Code: "55P03", Message: "lock_not_available"}},
		{"serialization failure", &pgconn.PgError{Code: "40001"}},
		{"admin shutdown", &pgconn.PgError{Code: "57P01"}},
		{"wrapped connection error", fmt.Errorf("settle: %w", errors.New("broken pipe"))},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if permanent(tc.err) {
				t.Errorf("permanent(%v) = true, want false — a fill would be written off "+
					"that the next attempt would have applied", tc.err)
			}
		})
	}
}

// A dropped event leaves nothing but its log line, so the line has to identify
// what was lost precisely enough to repair it by hand.
func TestDescribeNamesTheOrdersInvolved(t *testing.T) {
	fill := describe(models.LedgerEvent{Fill: &models.Trade{
		Market: "BTC-USD", RestingOrderID: 41, IncomingOrderID: 42,
		Quantity: 100_000_000, Price: 4_500_000,
	}})
	for _, want := range []string{"BTC-USD", "41", "42", "100000000", "4500000"} {
		if !contains(fill, want) {
			t.Errorf("describe(fill) = %q, missing %q", fill, want)
		}
	}

	cancel := describe(models.LedgerEvent{Cancel: &models.OrderCancel{Market: "BTC-USD", OrderID: 7}})
	for _, want := range []string{"BTC-USD", "7"} {
		if !contains(cancel, want) {
			t.Errorf("describe(cancel) = %q, missing %q", cancel, want)
		}
	}
}

func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) &&
		(haystack == needle || len(needle) == 0 ||
			indexOf(haystack, needle) >= 0)
}

func indexOf(haystack, needle string) int {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}
