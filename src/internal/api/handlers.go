package api

// import (
// 	"encoding/json"
// 	"fmt"
// 	"net/http"
//
// 	"github.com/ADHFMZ7/crypto-exchange/internal/models"
// )

// User handlers

// func UserPostHandler(w http.ResponseWriter, r *http.Request) {
//
// 	var user models.User
//
// 	err := json.NewDecoder(r.Body).Decode(&user)
// 	if err != nil {
// 		http.Error(w, "Failed to read request body", http.StatusBadRequest)
// 		return
// 	}
//
// 	// fmt.Println(w, r.Header)
// 	fmt.Fprintln(w, user)
// }
//
// func UserGetHandler(w http.ResponseWriter, r *http.Request) {
//
// 	fmt.Fprintf(w, "Hello from Go API!")
// }
//
// func WalletHandler(w http.ResponseWriter, r *http.Request) {
//
// 	fmt.Fprintf(w, "Hello from Go API!")
// }
//
// func DepositHandler(w http.ResponseWriter, r *http.Request) {
//
// 	fmt.Fprintf(w, "Hello from Go API!")
// }
//
// func OrderHandler(w http.ResponseWriter, r *http.Request) {
// 	// Post request
// 	// {
// 	//   "side": (buy or sell)
// 	//   "price":
// 	//   "size":
// 	// }
//
// }
//
// func OrderbookHandler(w http.ResponseWriter, r *http.Request) {
//
// 	fmt.Fprintf(w, "Hello from Go API!")
// }
