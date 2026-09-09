package stores

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

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

Set TEST_DATABASE_URL to run them. It must point at a database that can be
wiped — the schema is dropped and rebuilt from sql/migrations on every run — and
its name must end in _test, which requireDisposable enforces before anything is
dropped. Without the variable, every test here skips.

	docker compose -f docker-compose.yml up -d db
	createdb crypto_exchange_test  # or any other database named *_test
	TEST_DATABASE_URL='postgres://...@localhost:5432/crypto_exchange_test?sslmode=disable' \
		go test ./internal/stores/
*/

var testPool *pgxpool.Pool

func TestMain(m *testing.M) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		// No database configured: every test calls newTestStore, which skips.
		os.Exit(m.Run())
	}

	if err := requireDisposable(databaseURL); err != nil {
		fmt.Fprintf(os.Stderr, "refusing to run: %v\n", err)
		os.Exit(1)
	}

	ctx := context.Background()

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "TEST_DATABASE_URL is set but unusable: %v\n", err)
		os.Exit(1)
	}

	if err := rebuildSchema(ctx, pool); err != nil {
		fmt.Fprintf(os.Stderr, "could not build the test schema: %v\n", err)
		os.Exit(1)
	}

	testPool = pool
	code := m.Run()
	pool.Close()
	os.Exit(code)
}

// requireDisposable refuses a database whose name does not end in _test.
//
// The next thing that happens is DROP SCHEMA public CASCADE. The doc comment
// above warns about that, but a warning is not a guard: pasting the development
// URL into TEST_DATABASE_URL once is all it takes, and the naming rule costs
// nothing to follow.
func requireDisposable(databaseURL string) error {
	parsed, err := url.Parse(databaseURL)
	if err != nil {
		return fmt.Errorf("TEST_DATABASE_URL is not a URL: %w", err)
	}

	name := strings.TrimPrefix(parsed.Path, "/")
	if !strings.HasSuffix(name, "_test") {
		return fmt.Errorf("TEST_DATABASE_URL points at %q, which is not named like a test database. "+
			"These tests drop and rebuild the whole schema, so the name must end in _test", name)
	}
	return nil
}

// rebuildSchema drops everything and replays the migrations in order.
//
// Applying the real migration files rather than a hand-written fixture is the
// point: a test schema that drifts from the one production runs would let a
// constraint bug pass here and fail there.
func rebuildSchema(ctx context.Context, pool *pgxpool.Pool) error {
	if _, err := pool.Exec(ctx, `DROP SCHEMA public CASCADE; CREATE SCHEMA public;`); err != nil {
		return err
	}

	paths, err := filepath.Glob(filepath.Join("..", "..", "sql", "migrations", "*.up.sql"))
	if err != nil {
		return err
	}
	if len(paths) == 0 {
		return fmt.Errorf("no migrations found — is the test running from internal/stores?")
	}
	sort.Strings(paths) // numeric prefixes make lexical order the right order

	for _, path := range paths {
		statements, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if _, err := pool.Exec(ctx, string(statements)); err != nil {
			return fmt.Errorf("%s: %w", filepath.Base(path), err)
		}
	}
	return nil
}

// newTestStore returns a TradeStore over an empty database, skipping the test
// when no database is configured.
func newTestStore(t *testing.T) *TradeStore {
	t.Helper()

	if testPool == nil {
		t.Skip("TEST_DATABASE_URL not set — skipping database integration test")
	}

	_, err := testPool.Exec(context.Background(),
		`TRUNCATE ledger_events, trades, orders, balances, users RESTART IDENTITY CASCADE`)
	if err != nil {
		t.Fatalf("could not clear tables: %v", err)
	}

	return &TradeStore{testPool}
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
