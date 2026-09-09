package services

import (
	"context"
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

	SettlementChan chan models.Trade
}

func NewTradeService(userStore *stores.UserStore, walletStore *stores.WalletStore, tradeStore *stores.TradeStore, registry *market.Registry, SChan chan models.Trade) *TradeService {

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

	for trade := range service.SettlementChan {

		// A dropped fill is a fill the book has already acted on and the ledger
		// will never record, so every message below names the orders involved:
		// it is the only trace left of what the two sides now disagree about.
		m, ok := service.MarketRegistry.BySymbol(trade.Market)
		if !ok {
			log.Printf("settlement: dropped fill on unknown market %q (orders %d/%d, %d @ %d)",
				trade.Market, trade.RestingOrderID, trade.IncomingOrderID, trade.Quantity, trade.Price)
			continue
		}

		quoteAmount, err := m.FillNotional(trade.Quantity, trade.Price)
		if err != nil {
			log.Printf("settlement: dropped fill on %s (orders %d/%d, %d @ %d): notional: %v",
				trade.Market, trade.RestingOrderID, trade.IncomingOrderID, trade.Quantity, trade.Price, err)
			continue
		}

		if err := service.TradeStore.Settle(ctx, trade, m.Base.Code, m.Quote.Code, quoteAmount); err != nil {
			log.Printf("settlement: dropped fill on %s (orders %d/%d, %d @ %d): settle: %v",
				trade.Market, trade.RestingOrderID, trade.IncomingOrderID, trade.Quantity, trade.Price, err)
			continue
		}

	}

	log.Println("settlement: worker stopped, no further fills will settle")
}
