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

	stores := stores.NewStores(dbpool)
	services := services.NewServices(stores, registry, SChan)

	// Before anything can be accepted. The books were just built empty, so any
	// order still resting in Postgres from a previous run is unmatchable and is
	// holding funds against something that no longer exists. Serving without
	// unwinding those would hand out a market whose depth disagrees with its
	// own ledger — so a flush that fails is a reason not to start, not a
	// warning to log and continue past.
	if err := services.Orders.CancelRestingOrders(context.Background()); err != nil {
		log.Fatalf("could not reconcile the order book with the database: %v\n", err)
	}

	mux := api.NewRouter(services)

	var h http.Handler = mux
	h = api.WithCORS(h) // global

	log.Print("starting server on ", config.Server.GetURL())
	log.Fatal(http.ListenAndServe(config.Server.GetURL(), h))
}
