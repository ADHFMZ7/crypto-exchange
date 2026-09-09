package services

import (
	"context"
	"errors"
	"log"

	"github.com/ADHFMZ7/crypto-exchange/internal/market"
	"github.com/ADHFMZ7/crypto-exchange/internal/models"
	"github.com/ADHFMZ7/crypto-exchange/internal/stores"
)

type TradeService struct {
	WalletStore *stores.WalletStore
	UserStore   *stores.UserStore
	TradeStore  *stores.TradeStore

	MarketRegistry *market.Registry

	SettlementChan chan models.LedgerEvent
}

func NewTradeService(userStore *stores.UserStore, walletStore *stores.WalletStore, tradeStore *stores.TradeStore, registry *market.Registry, SChan chan models.LedgerEvent) *TradeService {

	service := &TradeService{
		WalletStore: walletStore,
		UserStore:   userStore,
		TradeStore:  tradeStore,

		MarketRegistry: registry,
		SettlementChan: SChan,
	}

	go service.SettlementWorker()

	return service
}

func (service *TradeService) SettlementWorker() {

	// TODO: Figure out where this should come from?
	ctx := context.Background()

	log.Println("settlement: worker started")

	for event := range service.SettlementChan {
		switch {
		case event.Fill != nil:
			service.settleFill(ctx, *event.Fill)
		case event.Cancel != nil:
			service.releaseCancelled(ctx, *event.Cancel)
		}
	}

	log.Println("settlement: worker stopped, no further events will be applied")
}

// settleFill applies one execution to the ledger.
//
// A fill that cannot be applied is dropped after logging: the book has already
// acted on it and will not agree with the ledger afterwards. Every message names
// the orders involved, because it is the only trace of what the two now
// disagree about.
func (service *TradeService) settleFill(ctx context.Context, trade models.Trade) {

	m, ok := service.MarketRegistry.BySymbol(trade.Market)
	if !ok {
		log.Printf("settlement: dropped fill on unknown market %q (orders %d/%d, %d @ %d)",
			trade.Market, trade.RestingOrderID, trade.IncomingOrderID, trade.Quantity, trade.Price)
		return
	}

	quoteAmount, err := m.FillNotional(trade.Quantity, trade.Price)
	if err != nil {
		log.Printf("settlement: dropped fill on %s (orders %d/%d, %d @ %d): notional: %v",
			trade.Market, trade.RestingOrderID, trade.IncomingOrderID, trade.Quantity, trade.Price, err)
		return
	}

	if err := service.TradeStore.Settle(ctx, trade, m.Base.Code, m.Quote.Code, quoteAmount); err != nil {
		log.Printf("settlement: dropped fill on %s (orders %d/%d, %d @ %d): settle: %v",
			trade.Market, trade.RestingOrderID, trade.IncomingOrderID, trade.Quantity, trade.Price, err)
		return
	}
}

// releaseCancelled returns what is left of a cancelled order's lock.
//
// Losing the race to a fill is normal rather than an error: the order reached a
// terminal status first, the book had already matched it, and there is nothing
// to release. It is logged at all only because the user asked for something
// that did not happen.
func (service *TradeService) releaseCancelled(ctx context.Context, cancel models.OrderCancel) {

	m, ok := service.MarketRegistry.BySymbol(cancel.Market)
	if !ok {
		log.Printf("settlement: cannot release order %d on unknown market %q",
			cancel.OrderID, cancel.Market)
		return
	}

	currency, err := m.Locks(cancel.Side)
	if err != nil {
		log.Printf("settlement: cannot release order %d: %v", cancel.OrderID, err)
		return
	}

	err = service.WalletStore.CancelOrder(ctx, cancel.OrderID, currency.Code)
	if errors.Is(err, stores.ErrNotCancellable) {
		log.Printf("settlement: order %d filled before its cancellation arrived", cancel.OrderID)
		return
	}
	if err != nil {
		log.Printf("settlement: could not release order %d on %s: %v",
			cancel.OrderID, cancel.Market, err)
		return
	}
}
