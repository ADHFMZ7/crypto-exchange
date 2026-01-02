package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/ADHFMZ7/crypto-exchange/internal/models"
	"github.com/ADHFMZ7/crypto-exchange/internal/services"
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
	mux.HandleFunc("GET /users/", router.UserGetHandler)
}

// Handlers below here
func (router *UserRouter) UserPostHandler(w http.ResponseWriter, r *http.Request) {
	// POST /users/ - register new user
	// Request body JSON format:
	// {
	//   "email": "
	//   "fullname": "",
	//   "password": ""
	// }
	// Responses:
	// 201 Created - user successfully registered
	// 400 Bad Request - invalid request data

	// TODO: Give user 10,000 starting balance in USD

	var userForm models.UserAuth

	err := json.NewDecoder(r.Body).Decode(&userForm)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	user, err := router.Services.Users.RegisterUser(
		ctx, userForm.Email, userForm.Fullname, userForm.Password)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	err = router.Services.Users.GiveStartingBalance(ctx, user.ID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	fmt.Fprintf(w, "User created with ID: %d", user.ID)
}

func (router *UserRouter) UserGetHandler(w http.ResponseWriter, r *http.Request) {
	// GET /users/{id} - get user info by ID
	// Responses:
	// 200 OK - user info returned
	// 400 Bad Request - invalid user ID format
	// 404 Not Found - user with given ID does not exist

	id := r.PathValue("id")
	idNum, err := strconv.ParseInt(id, 10, 64)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	user, err := router.Services.Users.GetUserByID(r.Context(), idNum)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(user)
}
