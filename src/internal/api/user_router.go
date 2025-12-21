package api

import (
	"fmt"
	"net/http"
	"encoding/json"

	"github.com/ADHFMZ7/crypto-exchange/internal/services"
	"github.com/ADHFMZ7/crypto-exchange/internal/models"
)

// TODO: Take in user service or all services
type UserRouter struct {
	Services *services.Services
}

func NewUserRouter(service *services.Services) *UserRouter {
	return &UserRouter{Services: service}
}

func (router *UserRouter) Register(mux *http.ServeMux) {

	mux.HandleFunc("POST /users/", router.UserPostHandler)

}


// Handlers below here

func (router *UserRouter) UserPostHandler(w http.ResponseWriter, r *http.Request) {

	var userform models.UserAuth

	err := json.NewDecoder(r.Body).Decode(&userform)
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}

	// fmt.Println(w, r.Header)
	fmt.Fprintln(w, userform)
}
