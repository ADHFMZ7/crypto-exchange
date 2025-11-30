package main

import (
	"net/http"
	"context"
	"log"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ADHFMZ7/crypto-exchange/config"
	"github.com/ADHFMZ7/crypto-exchange/api"
)

func main() {

	config := config.New()

	dbpool, err := pgxpool.New(context.Background(), config.DB.URL)
	if err != nil {
		log.Fatalf("Unable to create connection pool: %v\n", err)
	}
	defer dbpool.Close()

	mux := http.NewServeMux()

	log.Print("starting server on ", config.Server.GetURL())

	mux.HandleFunc("/", api.HelloHandler)

	mux.HandleFunc("POST /users", api.UserPostHandler)

	err = http.ListenAndServe(config.Server.GetURL(), mux)
	if err != nil {
		log.Fatal(err)
	}
}


