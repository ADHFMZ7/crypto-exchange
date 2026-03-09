package stores

import (
	"context"

	"github.com/ADHFMZ7/crypto-exchange/internal/models"
	"github.com/jackc/pgx/v5/pgxpool"
)

type WalletStore struct {
	pool *pgxpool.Pool
}

func (store *WalletStore) GetByUserID(ctx context.Context, userID int64) (*models.Wallet, error) {
	var wallet models.Wallet
	wallet.UserID = userID

	rows, err := store.pool.Query(ctx,
		`SELECT id, user_id, currency, balance FROM balances WHERE user_id = $1`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var balance models.Balance
		err := rows.Scan(&balance.ID, &balance.UserID, &balance.Currency, &balance.Amount)
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
		`SELECT balance FROM balances WHERE user_id = $1 AND currency = $2`,
		userID, currency,
	).Scan(&balance)
	if err != nil {
		return -1, err
	}

	return balance, nil
}

func (store *WalletStore) ModfyBalance(ctx context.Context, userID, newBalance int64) error {

	_, err := store.pool.Exec(ctx,
		`UPDATE balances SET balance = $1 WHERE user_id = $2`,
		newBalance, userID)

	return err
}
