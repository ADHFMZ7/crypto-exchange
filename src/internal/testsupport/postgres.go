// Package testsupport gives integration tests a real Postgres to run against.
//
// The invariants worth testing in this codebase live in the schema and in
// Postgres' own statement semantics — the non-negative CHECKs, the rule that a
// CHECK is evaluated against a proposed insert row before ON CONFLICT
// arbitration, FOR UPDATE ordering. A fake would reimplement those in Go and
// then assert the reimplementation.
package testsupport

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Tables every test wants emptied between cases, children first.
//
// Listed rather than discovered, so adding a table and forgetting it here fails
// loudly on the next run instead of leaking rows between tests.
var tables = []string{"ledger_events", "trades", "orders", "balances", "users"}

// Pool returns a pool scoped to its own schema, or skips the test when no
// database is configured.
//
// Each package gets its own schema because `go test ./...` runs package
// binaries concurrently: two of them rebuilding the same schema would drop each
// other's tables mid-run, and the failures would look like anything but that.
// The schema name is the package's, so a leftover one is self-explaining.
func Pool(t *testing.T, schema string) *pgxpool.Pool {
	t.Helper()

	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set — skipping database integration test")
	}
	if err := requireDisposable(url); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	name := "test_" + schema

	config, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatalf("TEST_DATABASE_URL is unusable: %v", err)
	}
	// Unqualified CREATE TABLE in the migrations lands in the first schema on
	// the search path, which is how the migrations build this schema rather
	// than public without being rewritten.
	config.ConnConfig.RuntimeParams["search_path"] = name

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatalf("connecting to the test database: %v", err)
	}

	if err := rebuild(ctx, pool, name); err != nil {
		pool.Close()
		t.Fatalf("building the %s test schema: %v", name, err)
	}

	t.Cleanup(pool.Close)
	return pool
}

// Truncate empties every table, so one test cannot see another's rows.
//
// RESTART IDENTITY keeps ids small and readable across a run, which matters
// when a failure message names one.
func Truncate(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()

	_, err := pool.Exec(context.Background(),
		fmt.Sprintf("TRUNCATE %s RESTART IDENTITY CASCADE", strings.Join(tables, ", ")))
	if err != nil {
		t.Fatalf("clearing tables: %v", err)
	}
}

// rebuild drops the schema and replays the migrations into it.
//
// Applying the real migration files rather than a hand-written fixture is the
// point: a test schema that drifted from the one production runs would let a
// constraint bug pass here and fail there.
func rebuild(ctx context.Context, pool *pgxpool.Pool, schema string) error {
	_, err := pool.Exec(ctx,
		fmt.Sprintf("DROP SCHEMA IF EXISTS %s CASCADE; CREATE SCHEMA %s;", schema, schema))
	if err != nil {
		return err
	}

	paths, err := filepath.Glob(filepath.Join("..", "..", "sql", "migrations", "*.up.sql"))
	if err != nil {
		return err
	}
	if len(paths) == 0 {
		return fmt.Errorf("no migrations found — is the test running from internal/<package>?")
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

// requireDisposable refuses a database whose name does not end in _test.
//
// These tests drop schemas. The doc comments warn about it, but a warning is
// not a guard: pasting the development URL into TEST_DATABASE_URL once is all
// it takes, and the naming rule costs nothing to follow.
func requireDisposable(databaseURL string) error {
	// The path is the database name; parsing the whole URL is unnecessary and
	// pgxpool has already validated it by the time this matters.
	cut := strings.LastIndex(databaseURL, "/")
	if cut < 0 {
		return fmt.Errorf("TEST_DATABASE_URL names no database")
	}
	name := databaseURL[cut+1:]
	if i := strings.IndexAny(name, "?"); i >= 0 {
		name = name[:i]
	}
	if !strings.HasSuffix(name, "_test") {
		return fmt.Errorf("TEST_DATABASE_URL points at %q, which is not named like a test "+
			"database. These tests drop and rebuild schemas, so the name must end in _test", name)
	}
	return nil
}
