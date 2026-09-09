package stores

import (
	"context"
	"errors"
	"fmt"

	"github.com/ADHFMZ7/crypto-exchange/internal/models"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type WalletStore struct {
	pool *pgxpool.Pool
}

func (store *WalletStore) GetByUserID(ctx context.Context, userID int64) (*models.Wallet, error) {
	var wallet models.Wallet
	wallet.UserID = userID

	// `locked` is selected as well as `available`: without it the column reads
	// zero for every currency, which makes an order that locked funds look like
	// it did nothing.
	rows, err := store.pool.Query(ctx,
		`SELECT id, user_id, currency, available, locked
		 FROM balances
		 WHERE user_id = $1
		 ORDER BY currency`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var balance models.Balance
		err := rows.Scan(&balance.ID, &balance.UserID, &balance.Currency, &balance.Available, &balance.Locked)
		if err != nil {
			return nil, err
		}
		wallet.Balances = append(wallet.Balances, balance)
	}

	return &wallet, nil
}

// TODO: Switch to use currency obj later?
func (store *WalletStore) GetUserBalance(ctx context.Context, userID int64, currency string) (int64, error) {

	var balance int64

	err := store.pool.QueryRow(ctx,
		`SELECT available FROM balances WHERE user_id = $1 AND currency = $2`,
		userID, currency,
	).Scan(&balance)
	if err != nil {
		return -1, err
	}

	return balance, nil
}

// ErrInsufficientFunds is returned when an adjustment would drive a balance
// below zero.
var ErrInsufficientFunds = errors.New("insufficient funds")

// AdjustBalance applies a signed delta to one currency's available balance.
//
// It is a delta rather than a new absolute value on purpose: reading a balance
// and writing back read+delta is a lost-update race, where two concurrent
// deposits both read the old value and one silently overwrites the other. Doing
// the arithmetic in the statement makes that unrepresentable.
//
// The upsert covers crediting a currency the user has never held — only USD is
// seeded at signup, so a first BTC deposit has no row to update. It relies on
// the (user_id, currency) unique constraint from migration 000004 as its
// conflict target.
//
// The WHERE on the update is the overdraft guard, and it is in the same
// statement as the debit so a withdrawal cannot pass a check and then be
// applied against a balance that moved underneath it.
func (store *WalletStore) AdjustBalance(ctx context.Context, userID int64, currency string, delta int64) error {

	// Update first. This cannot be folded into the insert below as a single
	// upsert: a withdrawal's proposed insert row carries a negative available,
	// which trips the balances_non_negative CHECK before Postgres reaches the
	// ON CONFLICT path, so every withdrawal would fail as a constraint error.
	tag, err := store.pool.Exec(ctx, `
		UPDATE balances
		SET available  = available + $3,
		    updated_at = now()
		WHERE user_id = $1
		  AND currency = $2
		  AND available + $3 >= 0
	`, userID, currency, delta)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 1 {
		return nil
	}

	// Nothing updated: either no such balance, or the guard rejected it. A
	// withdrawal has nothing to draw on in both cases.
	if delta < 0 {
		return ErrInsufficientFunds
	}

	// A credit in a currency the user has never held creates the row — only USD
	// is seeded at signup, so a first BTC deposit lands here. ON CONFLICT covers
	// the race where a concurrent deposit created the row in between.
	_, err = store.pool.Exec(ctx, `
		INSERT INTO balances (user_id, currency, available)
		VALUES ($1, $2, $3)
		ON CONFLICT (user_id, currency) DO UPDATE
		SET available  = balances.available + $3,
		    updated_at = now()
	`, userID, currency, delta)

	return err
}

func (store *WalletStore) PlaceOrder(ctx context.Context,
	userID int64, debit_currency string, debit_amount, quantity, price int64, side, market string) (int64, error) {
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return -1, err
	}
	defer tx.Rollback(ctx) // no-op if already committed

	var available int64
	var locked int64
	var orderID int64

	err = tx.QueryRow(ctx, `
        UPDATE balances
        SET
            available = available - $1,
            locked    = locked + $1,
            updated_at = now()
        WHERE user_id = $2
          AND currency = $3
          AND available >= $1
        RETURNING available, locked
    `, debit_amount, userID, debit_currency).Scan(&available, &locked)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return -1, errors.New("Insufficient funds")
		}
		return -1, err
	}

	// locked_remaining starts at the full debit: settlement decrements it as
	// fills release the lock, and needs a per-order figure because
	// balances.locked is aggregated across every order the user holds.
	err = tx.QueryRow(ctx, `
		INSERT INTO orders (user_id, quantity, price_each, side, market, status, locked_remaining)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id
	`, userID, quantity, price, side, market, "open", debit_amount).Scan(&orderID)

	if err != nil {
		return -1, err
	}

	return orderID, tx.Commit(ctx)
}

// ErrNotCancellable is returned when an order reached a terminal status before
// the cancellation got to it.
var ErrNotCancellable = errors.New("order is no longer open")

// CancelOrder marks a resting order cancelled and returns its unspent lock.
//
// This is the mirror of PlaceOrder: that one moves available into locked and
// opens the order, this one moves whatever is left back and closes it.
func (store *WalletStore) CancelOrder(ctx context.Context, orderID int64, lockedCurrency string) error {
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) // no-op if already committed

	if _, err := releaseOrder(ctx, tx, orderID, lockedCurrency); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// OrderRelease names one order and the currency its lock is held in. The
// currency comes from the caller because which one it is depends on the side
// and the market, and neither is a store's business.
type OrderRelease struct {
	OrderID  int64
	Currency string
}

// CancelRestingOrders cancels many orders in one transaction and returns how
// much was released, by currency.
//
// This is the boot-time flush. The order book lives in memory and Postgres does
// not, so a restart leaves rows resting that nothing can ever match, with their
// funds locked against orders that no longer exist anywhere. Cancelling them is
// what makes the two states agree again — the cheap end of the recovery
// spectrum, chosen over rebuilding the book because it cannot be wrong.
//
// One transaction rather than one per order: a half-applied flush would leave
// the exchange serving with some phantom orders still locked, and at boot there
// is nothing to contend with. An order that is already terminal is skipped
// rather than failing the batch, so the flush is safe to run twice.
func (store *WalletStore) CancelRestingOrders(ctx context.Context, releases []OrderRelease) (map[string]int64, error) {
	freed := map[string]int64{}
	if len(releases) == 0 {
		return freed, nil
	}

	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	for _, release := range releases {
		released, err := releaseOrder(ctx, tx, release.OrderID, release.Currency)
		if errors.Is(err, ErrNotCancellable) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("releasing order %d: %w", release.OrderID, err)
		}
		freed[release.Currency] += released
	}

	return freed, tx.Commit(ctx)
}

// releaseOrder closes one order and returns what it still had locked.
//
// The status test lives inside the statement rather than in a read beforehand,
// so a fill settling concurrently is resolved by Postgres rather than by
// ordering luck. Whichever transaction commits second finds the row already
// terminal and changes nothing: a filled order stays filled, and a lock can
// never be released twice. That matters more than it looks — locked_remaining
// is what settlement decrements, so a double release would drive a live order's
// lock negative and strand every later fill against the CHECK.
//
// The CTE exists because RETURNING reports post-update values, and the amount
// to give back is the value from before. `held` captures it under FOR UPDATE;
// the UPDATE then zeroes the column and hands that captured figure back.
func releaseOrder(ctx context.Context, tx pgx.Tx, orderID int64, lockedCurrency string) (int64, error) {
	var userID, released int64

	err := tx.QueryRow(ctx, `
		WITH held AS (
			SELECT id, user_id, locked_remaining
			FROM orders
			WHERE id = $1
			  AND status IN ($3, $4)
			FOR UPDATE
		), cancelled AS (
			UPDATE orders
			SET status           = $2,
			    locked_remaining = 0
			FROM held
			WHERE orders.id = held.id
			RETURNING held.user_id, held.locked_remaining
		)
		SELECT user_id, locked_remaining FROM cancelled
	`, orderID, models.OrderCancelled, models.OrderOpen, models.OrderPartiallyFilled).
		Scan(&userID, &released)

	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrNotCancellable
	}
	if err != nil {
		return 0, err
	}

	// A fully filled lock leaves nothing to return, and moveBalance would
	// rather not be asked to move zero.
	if released > 0 {
		if err := moveBalance(ctx, tx, userID, lockedCurrency, released, -released); err != nil {
			return 0, err
		}
	}

	return released, nil
}
