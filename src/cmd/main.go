package main

import (
	"log"
	"net/http"

	"github.com/ADHFMZ7/crypto-exchange/api"
	"github.com/ADHFMZ7/crypto-exchange/config"
	"github.com/ADHFMZ7/crypto-exchange/internal/db"
	"github.com/ADHFMZ7/crypto-exchange/internal/store"
)

func main() {

	config := config.New()

	dbpool, err := db.NewPool(config.DB.URL)
	if err != nil {
		log.Fatalf("Unable to create connection pool: %v\n", err)
	}
	defer dbpool.Close()

	stores := store.NewStores(dbpool)

	services := api.NewServices(stores)

	mux := http.NewServeMux()

	log.Print("starting server on ", config.Server.GetURL())

	mux.HandleFunc("POST /users", api.UserPostHandler)

	err = http.ListenAndServe(config.Server.GetURL(), mux)
	if err != nil {
		log.Fatal(err)
	}
}
