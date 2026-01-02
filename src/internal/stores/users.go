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
		"SELECT id, fullname, email FROM users WHERE id = ($1)",
		id).Scan(&user.ID, &user.Fullname, &user.Email)
	if err != nil {
		return nil, err
	}

	return &user, nil
}

func (store *UserStore) GetByEmail(ctx context.Context, email string) (*models.UserAuth, error) {

	var user models.UserAuth

	err := store.db.QueryRow(ctx,
		"SELECT id, fullname, email, hashed_password FROM users WHERE email = ($1)",
		email).Scan(&user.ID, &user.Fullname, &user.Email, &user.Password)
	if err != nil {
		return nil, err
	}

	return &user, nil
}

func (store *UserStore) GiveBalance(ctx context.Context, userID int64, currency string, amount int64) error {
	// Give new user a starting balance

	_, err := store.db.Exec(ctx,
		`INSERT INTO balance (user_id, currency, balance) VALUES ($1, $2, $3)`,
		userID,
		currency,
		amount, // in cents
	)
	return err
}
