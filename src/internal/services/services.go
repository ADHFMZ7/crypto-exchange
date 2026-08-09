package services

import (
	"github.com/ADHFMZ7/crypto-exchange/internal/market"
	"github.com/ADHFMZ7/crypto-exchange/internal/stores"
)

type Services struct {
	Users   *UserService
	Wallets *WalletService
	// Trades  *TradeService
	Orders *OrderService
}

func NewServices(stores *stores.Stores, registry *market.Registry) *Services {

	return &Services{
		Users:   NewUserService(stores.Users),
		Wallets: NewWalletService(stores.Wallets, stores.Users, registry),
		// Trades:  NewTradeService(stores.Users, stores.Wallets, registry),
		Orders: NewOrderService(stores.Wallets, stores.Orders, registry),
	}
}
