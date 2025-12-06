package store

import (
	"context"

	"github.com/ADHFMZ7/crypto-exchange/internal/auth"
	"github.com/ADHFMZ7/crypto-exchange/internal/models"
	"github.com/jackc/pgx/v5/pgxpool"
)

type UserStore struct {
	db *pgxpool.Pool
}

func (store *UserStore) CreateUser(user models.User) error {

	hashed_password := auth.HashPassword(user.Password)

	_, err := store.db.Exec(context.Background(),
		"INSERT INTO users (fname, lname, email, password_hash) VALUES ($1, $2, $3, $4)",
		user.Fname,
		user.Lname,
		user.Email,
		hashed_password,
	)
	return err

}
