package services

import (
	"sync/atomic"

	"github.com/ADHFMZ7/crypto-exchange/internal/orderbook"
	"github.com/ADHFMZ7/crypto-exchange/internal/stores"
)

type TradeService struct {
	WalletStore *stores.WalletStore
	UserStore   *stores.UserStore

	Orderbook *orderbook.Orderbook

	RQueues map[string]chan Request

	nextOrderID atomic.Int64
}

func NewTradeService(userStore *stores.UserStore, walletStore *stores.WalletStore) *TradeService {

	symbols := []string{"BTC-USD"}
	channels := map[string]chan Request{}

	service := &TradeService{
		WalletStore: walletStore,
		UserStore:   userStore,
		Orderbook:   orderbook.NewOrderbook(),

		RQueues: channels,
	}

	for _, symbol := range symbols {
		channels[symbol] = make(chan Request, 1024)
		go service.StartWorker(channels[symbol])
	}

	return service
}

func (service *TradeService) StartWorker(channel chan Request) {

	for request := range channel {

		id := orderbook.OrderID(request.OrderID)

		if request.Type == Cancel {
			service.Orderbook.Cancel(id)
			continue
		}

		shares := orderbook.Shares(request.Shares)
		price := orderbook.Price(request.Price)

		switch request.Type {
		case LimitBuy:
			service.Orderbook.LimitBuy(id, shares, price)
		case LimitSell:
			service.Orderbook.LimitSell(id, shares, price)
		}

	}

}

func (service *TradeService) NextOrderID() int64 {
	return service.nextOrderID.Add(1)
}

type RequestType int

const (
	LimitBuy RequestType = iota
	LimitSell
	Cancel
)

type Request struct {
	Type    RequestType
	OrderID int64
	Price   int64
	Shares  int64
}
