package services

// import (
// 	"context"
// 	"sync/atomic"

// 	"github.com/ADHFMZ7/crypto-exchange/internal/market"
// 	"github.com/ADHFMZ7/crypto-exchange/internal/orderbook"
// 	"github.com/ADHFMZ7/crypto-exchange/internal/stores"
// )

// type TradeService struct {
// 	WalletStore *stores.WalletStore
// 	UserStore   *stores.UserStore

// 	MarketRegistry *market.Registry

// 	Orderbook *orderbook.Orderbook

// 	RQueues map[string]chan Request

// 	nextOrderID atomic.Int64
// }

// func NewTradeService(userStore *stores.UserStore, walletStore *stores.WalletStore, registry *market.Registry) *TradeService {

// 	symbols := []string{"BTC-USD"}

// 	service := &TradeService{
// 		WalletStore: walletStore,
// 		UserStore:   userStore,

// 		MarketRegistry: registry,

// 		Orderbook: orderbook.NewOrderbook(),

// 	}

// 	return service
// }

// func (service *TradeService) LimitSell(
// 	ctx context.Context,
// 	userid int64,
// 	market market.Market,
// 	volume int64,
// 	limit int64,
// ) {
// 	// User with id userid is making a request to sell volume units of c1 at price limit for each unit of c2

// 	currency := market.Base

// 	order_id, err := service.WalletStore.PlaceOrder(ctx, userid, currency.Code, volume, limit, "sell", market.Symbol)
// 	// If these funds exist, lock them and return true. After this point, funds are already validated

// 	if err != nil {
// 		println("ERROR: LimitSell Failed!")
// 		return
// 	}

// 	// We can now safely add to the correct order queue
// 	req := Request{
// 		Type:    LimitSell,
// 		OrderID: order_id,
// 		Price:   limit,
// 		Shares:  volume,
// 	}

// 	service.RQueues[market.Symbol] <- req
// 	// Consumed by orderbook worker

// 	// TODO: Maybe add more error checking later?

// }

// func (service *TradeService) LimitBuy(
// 	ctx context.Context,
// 	userid int64,
// 	market market.Market,
// 	volume int64,
// 	limit int64,
// ) {

// 	currency := market.Quote

// 	order_id, err := service.WalletStore.PlaceOrder(ctx, userid, currency.Code, volume, limit, "buy", market.Symbol)

// 	if err != nil {
// 		println("ERROR: LimitBuy Failed!")
// 		return
// 	}

// 	req := Request{
// 		Type:    LimitBuy,
// 		OrderID: order_id,
// 		Price:   limit,
// 		Shares:  volume,
// 	}

// 	service.RQueues[market.Symbol] <- req

// }
