package services

import (
	"github.com/ADHFMZ7/crypto-exchange/internal/market"
	"github.com/ADHFMZ7/crypto-exchange/internal/models"
	"github.com/ADHFMZ7/crypto-exchange/internal/stores"
	"github.com/ADHFMZ7/crypto-exchange/internal/stream"
)

type Services struct {
	Users   *UserService
	Wallets *WalletService
	Trades  *TradeService
	Orders  *OrderService
}

// NewServices wires everything and starts the goroutines that run the exchange.
//
// hub is passed in rather than assigned afterwards because the workers start
// here: setting the field on the way out would have those goroutines reading it
// while this one writes it.
func NewServices(stores *stores.Stores, registry *market.Registry,
	SChan chan models.LedgerEvent, hub *stream.Hub) *Services {

	return &Services{
		Users:   NewUserService(stores.Users),
		Wallets: NewWalletService(stores.Wallets, stores.Users, registry),
		Trades:  NewTradeService(stores.Users, stores.Wallets, stores.Trades, stores.Outbox, registry, SChan, hub),
		Orders:  NewOrderService(stores.Wallets, stores.Orders, registry, SChan, hub),
	}
}
