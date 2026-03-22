package services

import (
	"context"
	"sync/atomic"

	"github.com/ADHFMZ7/crypto-exchange/internal/models"
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

func (service *TradeService) LimitSell(
	ctx context.Context,
	userid int64, 
	sell_currency, buy_currency string,
	volume int64,
	limit int64, 
	) {
	// User with id userid is making a request to sell volume units of c1 at price limit for each unit of c2

	market := GetMarketSymbol(sell_currency, buy_currency)

	id, err := service.WalletStore.PlaceOrder(ctx, userid, sell_currency, volume, limit, "sell", market)
	// If these funds exist, lock them and return true. After this point, funds are already validated

	if err != nil {
		return
	}

	// We can now safely add to the correct order queue
	req := Request{
		Type: LimitSell,
		OrderID: id,
		Price: limit,
		Shares: volume,
	}

	service.RQueues[market] <- req

	// now consume the result somehow


}

func (service *TradeService) LimitBuy(
	userid int64,
	c1, c2 models.Currency,
	volume int64,
	limit int64,
) {
}


func (service *TradeService) NextOrderID() int64 {
	return service.nextOrderID.Add(1)
}

func GetMarketSymbol(c1, c2 string) string {
    if c1 < c2 {
        return c1 + "-" + c2
    }
    return c2 + "-" + c1
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
