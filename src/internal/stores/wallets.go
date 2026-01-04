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
		`SELECT id, user_id, currency, amount FROM balances WHERE user_id = $1`,
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
