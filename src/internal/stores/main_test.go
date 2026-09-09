package stores

import (
	"context"
	"testing"

	"github.com/ADHFMZ7/crypto-exchange/internal/testsupport"
	"github.com/jackc/pgx/v5/pgxpool"
)

/*
Integration tests for the settlement path.

These need a real Postgres and cannot be faked. The invariants under test live
in the schema and in Postgres' own statement semantics — balances_non_negative,
orders_filled_within_quantity, and the rule that a CHECK is evaluated against a
proposed insert row before ON CONFLICT arbitration. An in-memory fake would
reimplement those in Go and then assert the reimplementation, which proves
nothing about the code that actually runs.

Set TEST_DATABASE_URL to run them; without it every test here skips. See
internal/testsupport for what it must point at and why each package gets its
own schema.

	docker compose -f docker-compose.yml up -d db
	createdb crypto_exchange_test  # or any other database named *_test
	TEST_DATABASE_URL='postgres://...@localhost:5432/crypto_exchange_test?sslmode=disable' \
		go test ./internal/stores/
*/

var testPool *pgxpool.Pool

// newTestStores prepares an empty schema and returns every store over it,
// skipping the test when no database is configured.
//
// One entry point rather than a pool a test may read before it is set: the
// stores are handed out already built, so there is no order to get wrong.
func newTestStores(t *testing.T) (*TradeStore, *WalletStore, *OutboxStore, *OrderStore) {
	t.Helper()

	testPool = testsupport.Pool(t, "stores")
	testsupport.Truncate(t, testPool)

	return &TradeStore{testPool}, &WalletStore{testPool}, &OutboxStore{testPool}, &OrderStore{testPool}
}

// newTestStore is newTestStores for the tests that only want the trade store.
func newTestStore(t *testing.T) *TradeStore {
	t.Helper()

	trades, _, _, _ := newTestStores(t)
	return trades
}

func seedUser(t *testing.T, email string) int64 {
	t.Helper()

	var id int64
	err := testPool.QueryRow(context.Background(),
		`INSERT INTO users (fullname, email, hashed_password) VALUES ($1, $2, 'x') RETURNING id`,
		email, email,
	).Scan(&id)
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}
	return id
}

func seedBalance(t *testing.T, userID int64, currency string, available, locked int64) {
	t.Helper()

	_, err := testPool.Exec(context.Background(),
		`INSERT INTO balances (user_id, currency, available, locked) VALUES ($1, $2, $3, $4)`,
		userID, currency, available, locked,
	)
	if err != nil {
		t.Fatalf("seed balance: %v", err)
	}
}

func seedOrder(t *testing.T, userID int64, side string, quantity, price, lockedRemaining int64) int64 {
	t.Helper()

	var id int64
	err := testPool.QueryRow(context.Background(), `
		INSERT INTO orders (user_id, quantity, price_each, side, market, status, locked_remaining)
		VALUES ($1, $2, $3, $4, 'BTC-USD', 'open', $5)
		RETURNING id
	`, userID, quantity, price, side, lockedRemaining).Scan(&id)
	if err != nil {
		t.Fatalf("seed order: %v", err)
	}
	return id
}

func readBalance(t *testing.T, userID int64, currency string) (available, locked int64) {
	t.Helper()

	err := testPool.QueryRow(context.Background(),
		`SELECT available, locked FROM balances WHERE user_id = $1 AND currency = $2`,
		userID, currency,
	).Scan(&available, &locked)
	if err != nil {
		t.Fatalf("read balance %d/%s: %v", userID, currency, err)
	}
	return available, locked
}

func readOrder(t *testing.T, orderID int64) (filled int64, status string, lockedRemaining int64) {
	t.Helper()

	err := testPool.QueryRow(context.Background(),
		`SELECT filled_quantity, status, locked_remaining FROM orders WHERE id = $1`,
		orderID,
	).Scan(&filled, &status, &lockedRemaining)
	if err != nil {
		t.Fatalf("read order %d: %v", orderID, err)
	}
	return filled, status, lockedRemaining
}
