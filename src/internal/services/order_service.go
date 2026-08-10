package services

import (
	"context"
	"errors"

	"github.com/ADHFMZ7/crypto-exchange/internal/market"
	"github.com/ADHFMZ7/crypto-exchange/internal/models"
	"github.com/ADHFMZ7/crypto-exchange/internal/orderbook"
	"github.com/ADHFMZ7/crypto-exchange/internal/stores"
)

// TODO: Put proper errrors

type OrderService struct {
	WalletStore *stores.WalletStore
	OrderStore  *stores.OrderStore

	Registry *market.Registry

	Orderbooks map[string]*orderbook.Orderbook
	RQueues    map[string]chan Request
}

func NewOrderService(walletStore *stores.WalletStore, orderStore *stores.OrderStore, registry *market.Registry) *OrderService {

	channels := map[string]chan Request{}

	service := &OrderService{
		WalletStore: walletStore,
		OrderStore:  orderStore,

		Registry: registry,

		Orderbooks: make(map[string]*orderbook.Orderbook),
		RQueues:    channels,
	}

	for _, m := range registry.Markets() {
		service.Orderbooks[m.Symbol] = orderbook.NewOrderbook()
		channels[m.Symbol] = make(chan Request, 1024)
		go service.StartWorker(channels[m.Symbol], m.Symbol)
	}

	return service
}

func (service *OrderService) StartWorker(channel chan Request, market string) {

	book, ok := service.Orderbooks[market]
	if !ok {
		return
	}

	for request := range channel {

		id := orderbook.OrderID(request.OrderID)

		if request.Type == Cancel {
			book.Cancel(id)
			continue
		}

		shares := orderbook.Shares(request.Shares)
		price := orderbook.Price(request.Price)

		switch request.Type {
		case LimitBuy:
			book.LimitBuy(id, shares, price)
		case LimitSell:
			book.LimitSell(id, shares, price)
		}

	}

}

// TODO: Find a place for this later
type OrderReq struct {
	Market   string `json:"market"`
	Side     string `json:"side"`
	Quantity int64  `json:"quantity"`
	Price    int64  `json:"price"`
}

func (service *OrderService) CreateOrder(ctx context.Context, userID int64, payload OrderReq) (int64, error) {

	m, ok := service.Registry.BySymbol(payload.Market)
	if !ok {
		return 0, errors.New("Market does not exist")
	}

	// BTC-USD
	// BTC is base
	// USD is quote

	// If we are doing a buy order, then we are	buying BTC
	// If we are doing a sell order, then we are selling BTC.
	// Price in both is cents per btc

	currency, amount, err := m.Spends(payload.Side, payload.Quantity, payload.Price)
	if err != nil {
		return 0, err
	}

	request_type, err := requestTypeFor(payload.Side)
	if err != nil {
		return 0, err
	}

	req_chan, ok := service.RQueues[m.Symbol]
	if !ok {
		return 0, errors.New("Channel does not exist")
	}

	order_id, err := service.WalletStore.PlaceOrder(ctx, userID, currency.Code, amount, payload.Quantity, payload.Price, payload.Side, m.Symbol)
	if err != nil {
		return 0, err
	}

	// // Consumed by orderbook worker
	// req_chan <- Request{
	// 	Type:    request_type,
	// 	OrderID: order_id,
	// 	Price:   payload.Price,
	// 	Shares:  payload.Quantity,
	// }

	select {
	case req_chan <- Request{
		Type:    request_type,
		OrderID: order_id,
		Price:   payload.Price,
		Shares:  payload.Quantity,
	}:
		return order_id, nil

	case <-ctx.Done():
		// Client is gone. Release the lock rather than leaving an order nobody knows about.
		// TODO: Implement ReleaseOrder in wallet store
		// if err := service.WalletStore.ReleaseOrder(ctx, order_id); err != nil {
		// 	log.Printf("orphaned order %d: funds locked, not queued: %v", order_id, err)
		// }
		return 0, ctx.Err()
	}

}

func (service *OrderService) GetOrdersByID(ctx context.Context, userID int64) (*models.Orders, error) {

	orders, err := service.OrderStore.GetByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}

	return orders, nil
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

func requestTypeFor(side string) (RequestType, error) {
	switch side {
	case "buy":
		return LimitBuy, nil
	case "sell":
		return LimitSell, nil
	}
	return 0, market.ErrInvalidSide
}

// func parseOrderType(raw string) (RequestType, bool) {
// 	switch strings.ToLower(strings.TrimSpace(raw)) {
// 	case "limit_buy", "buy":
// 		return LimitBuy, true
// 	case "limit_sell", "sell":
// 		return LimitSell, true
// 	case "cancel":
// 		return Cancel, true
// 	default:
// 		return LimitBuy, false
// 	}
// }
