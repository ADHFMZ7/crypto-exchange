package stores

import (
	"github.com/jackc/pgx/v5/pgxpool"
)

type Stores struct {
	Users   *UserStore
	Wallets *WalletStore
}

func NewStores(pool *pgxpool.Pool) *Stores {
	return &Stores{
		Users:   &UserStore{pool},
		Wallets: &WalletStore{pool},
	}
}
