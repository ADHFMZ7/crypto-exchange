package services

import (
	"github.com/ADHFMZ7/crypto-exchange/internal/market"
	"github.com/ADHFMZ7/crypto-exchange/internal/models"
	"github.com/ADHFMZ7/crypto-exchange/internal/stores"
)

type Services struct {
	Users   *UserService
	Wallets *WalletService
	Trades  *TradeService
	Orders  *OrderService
}

func NewServices(stores *stores.Stores, registry *market.Registry, SChan chan models.Trade) *Services {

	return &Services{
		Users:   NewUserService(stores.Users),
		Wallets: NewWalletService(stores.Wallets, stores.Users, registry),
		Trades:  NewTradeService(stores.Users, stores.Wallets, stores.Trades, registry, SChan),
		Orders:  NewOrderService(stores.Wallets, stores.Orders, registry, SChan),
	}
}
