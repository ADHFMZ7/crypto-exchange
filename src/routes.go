package main

import (
	"fmt"
	"net/http"
)

func helloHandler(w http.ResponseWriter, r *http.Request) {

	println(r.GetBody())

	fmt.Fprintf(w, "Hello from Go API!")
}

func userHandler(w http.ResponseWriter, r *http.Request) {

}

func Handler(w http.ResponseWriter, r *http.Request) {

}

func routes() {
	http.HandleFunc("/", helloHandler)

	http.HandleFunc("/user/{id}", userHandler)
	http.HandleFunc("/wallets", Handler)
	http.HandleFunc("/deposit", Handler)
	http.HandleFunc("/orders", Handler)
	http.HandleFunc("/orderbook", Handler)

	http.ListenAndServe(":8080", nil)
}
