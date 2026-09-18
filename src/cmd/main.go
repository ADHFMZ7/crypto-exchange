package main

import (
	"context"
	"log"
	"net/http"

	"github.com/ADHFMZ7/crypto-exchange/config"
	"github.com/ADHFMZ7/crypto-exchange/internal/api"
	"github.com/ADHFMZ7/crypto-exchange/internal/db"
	"github.com/ADHFMZ7/crypto-exchange/internal/market"
	"github.com/ADHFMZ7/crypto-exchange/internal/models"
	"github.com/ADHFMZ7/crypto-exchange/internal/services"
	"github.com/ADHFMZ7/crypto-exchange/internal/stores"
	"github.com/ADHFMZ7/crypto-exchange/internal/stream"
)

func main() {

	config := config.New()

	dbpool, err := db.NewPool(config.DB.URL)
	if err != nil {
		log.Fatalf("Unable to create connection pool: %v\n", err)
	}
	defer dbpool.Close()

	currencies, markets := market.Default()
	registry, _ := market.NewMarketRegistry(currencies, markets)
	SChan := make(chan models.LedgerEvent, 1024)

	// The live feed. Settlement announces executions onto it, the matching
	// workers announce depth, the stream endpoint reads from it, and none of
	// them can block another: a client that stops reading is dropped rather
	// than waited for.
	hub := stream.NewHub()

	stores := stores.NewStores(dbpool)
	services := services.NewServices(stores, registry, SChan, hub)

	// Recovery, in this order and only this order.
	//
	// First pay what the ledger is owed: effects the previous run's engine
	// produced and never managed to apply. Then unwind whatever is still
	// resting, because the books were just built empty and nothing in Postgres
	// can be matched against them any more.
	//
	// The reverse order corrupts: the flush zeroes locked_remaining, and a
	// pending fill drawing down a lock that is already zero is refused by
	// orders_locked_remaining_non_negative. A process that cannot establish a
	// consistent starting state should not serve, so both are fatal.
	ctx := context.Background()

	if err := services.Trades.ReplayPending(ctx); err != nil {
		log.Fatalf("could not apply ledger events owed from a previous run: %v\n", err)
	}

	if err := services.Orders.CancelRestingOrders(ctx); err != nil {
		log.Fatalf("could not reconcile the order book with the database: %v\n", err)
	}

	mux := api.NewRouter(services, hub)

	var h http.Handler = mux
	h = api.WithCORS(h) // global

	log.Print("starting server on ", config.Server.GetURL())
	log.Fatal(http.ListenAndServe(config.Server.GetURL(), h))
}
