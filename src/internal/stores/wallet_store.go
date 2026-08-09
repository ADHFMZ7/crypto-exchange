package stores

import (
	"context"
	"errors"

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

	err = tx.QueryRow(ctx, `
		INSERT INTO orders (user_id, quantity, price_each, side, market, status)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id
	`, userID, quantity, price, side, market, "open").Scan(&orderID)

	if err != nil {
		return -1, err
	}

	return orderID, tx.Commit(ctx)
}
