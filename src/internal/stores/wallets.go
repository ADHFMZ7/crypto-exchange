package stores

import (
	"context"
	"errors"
	"fmt"

	"github.com/ADHFMZ7/crypto-exchange/internal/models"
	"github.com/jackc/pgx/v5/pgxpool"
    "github.com/jackc/pgx/v5"
)

type WalletStore struct {
	pool *pgxpool.Pool
}

func (store *WalletStore) GetByUserID(ctx context.Context, userID int64) (*models.Wallet, error) {
	var wallet models.Wallet
	wallet.UserID = userID

	rows, err := store.pool.Query(ctx,
		`SELECT id, user_id, currency, available FROM balances WHERE user_id = $1`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var balance models.Balance
		err := rows.Scan(&balance.ID, &balance.UserID, &balance.Currency, &balance.Available)
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

func (store *WalletStore) ModifyBalance(ctx context.Context, userID, newBalance int64) error {

	_, err := store.pool.Exec(ctx,
		`UPDATE balances SET balance = $1 WHERE user_id = $2`,
		newBalance, userID)

	return err
}


func (store *WalletStore) LockFundsIfExist(ctx context.Context, userID int64, currency models.Currency, amount int64) error {

	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) // no-op if already committed

    var available int64
    var locked int64

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
    `, amount, userID, currency).Scan(&available, &locked)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
            return fmt.Errorf("insufficient funds")
        }
        return err
	}

	_, err = tx.Exec(ctx, `
        INSERT INTO orders (user_id, currency, amount, status)
        VALUES ($1, $2, $3, 'OPEN')
    `, userID, currency, amount)
    if err != nil {
        return err
    }
	
	return tx.Commit(ctx)

}
