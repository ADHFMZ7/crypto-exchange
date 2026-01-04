package main

import (
	"log"
	"net/http"

	"github.com/ADHFMZ7/crypto-exchange/config"
	"github.com/ADHFMZ7/crypto-exchange/internal/api"
	"github.com/ADHFMZ7/crypto-exchange/internal/db"
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

	stores := stores.NewStores(dbpool)
	services := services.NewServices(stores)
	mux := api.NewRouter(services)

	var h http.Handler = mux
	h = api.WithCORS(h) // global

	log.Print("starting server on ", config.Server.GetURL())
	log.Fatal(http.ListenAndServe(config.Server.GetURL(), h))
}
