package store

import (
	"github.com/jackc/pgx/v5/pgxpool"
)

type Stores struct {
	Users *UserStore
}

func NewStores(pool *pgxpool.Pool) *Stores {
	return &Stores{
		Users: &UserStore{pool},
	}
}
