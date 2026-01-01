package stores

import (
	"context"

	"github.com/ADHFMZ7/crypto-exchange/internal/models"
	"github.com/jackc/pgx/v5/pgxpool"
)

type UserStore struct {
	db *pgxpool.Pool
}

func (store *UserStore) CreateUser(ctx context.Context, fullname, email, hashed_password string) (*models.User, error) {

	var user models.User

	err := store.db.QueryRow(ctx,
		`INSERT INTO users (fullname, email, hashed_password) VALUES ($1, $2, $3) RETURNING id, fullname, email`,
		fullname,
		email,
		hashed_password,
	).Scan(&user.ID, &user.Fullname, &user.Email)
	if err != nil {
		return nil, err
	}

	return &user, err
}

func (store *UserStore) GetByID(ctx context.Context, id int64) (*models.User, error) {

	var user models.User

	err := store.db.QueryRow(ctx,
		"SELECT (id, fullname, email) FROM users WHERE id == (?)",
		id).Scan(&user.ID, &user.Fullname, &user.Email)
	if err != nil {
		return nil, err
	}

	return &user, nil
}

func (store *UserStore) GiveBalance(ctx context.Context, userID int64, currency string, amount int64) error {
	// Give new user a starting balance

	_, err := store.db.Exec(ctx,
		`INSERT INTO balances (user_id, currency, balance) VALUES ($1, $2, $3)`,
		userID,
		currency,
		amount, // in cents
	)
	return err
}
