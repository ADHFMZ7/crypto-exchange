package services

import "github.com/ADHFMZ7/crypto-exchange/internal/stores"

type Services struct {
	Users   *UserService
	Wallets *WalletService
	Trades  *TradeService
}

func NewServices(stores *stores.Stores) *Services {

	return &Services{
		Users:   NewUserService(stores.Users),
		Wallets: NewWalletService(stores.Wallets, stores.Users),
		Trades:  NewTradeService(stores.Users, stores.Wallets),
	}
}
