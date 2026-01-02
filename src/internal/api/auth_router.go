package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/ADHFMZ7/crypto-exchange/internal/auth"
	"github.com/ADHFMZ7/crypto-exchange/internal/models"
	"github.com/ADHFMZ7/crypto-exchange/internal/services"
)

type AuthRouter struct {
	Services *services.Services
}

func NewAuthRouter(service *services.Services) *AuthRouter {
	return &AuthRouter{Services: service}
}

func (router *AuthRouter) Register(mux *http.ServeMux) {

	mux.Handle(
		"OPTIONS /auth/",
		withCORS(http.HandlerFunc(emptyHandler)),
	)
	mux.Handle(
		"POST /auth/login",
		withCORS(http.HandlerFunc(router.LoginHandler)),
	)
	mux.Handle(
		"POST /auth/logout",
		withCORS(http.HandlerFunc(router.LogoutHandler)),
	)
}

// Handlers below here
func (router *AuthRouter) LoginHandler(w http.ResponseWriter, r *http.Request) {
	// POST /auth/login - user login
	// Request body JSON format:
	// {
	//   "email": "
	//   "password": ""
	// }
	// Responses:
	// 200 OK - user successfully logged in
	// 400 Bad Request - invalid request data
	// 401 Unauthorized - invalid email or password

	var user models.UserAuth
	ctx := r.Context()

	err := json.NewDecoder(r.Body).Decode(&user)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// validate email and password
	fmt.Println("Getting user by email:", user.Email)
	storedUser, err := router.Services.Users.GetUserByEmail(ctx, user.Email)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}
	fmt.Println("Retrieved user:", storedUser)
	// Check password
	valid := auth.CheckPasswordHash(user.Password, storedUser.Password)
	if !valid {
		http.Error(w, "Invalid email or password", http.StatusUnauthorized)
		return
	}

	token, err := auth.GenerateJWT(strconv.Itoa(int(storedUser.ID)), 0)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Return token to user
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"token": token})
}

func (router *AuthRouter) LogoutHandler(w http.ResponseWriter, r *http.Request) {
	// POST /auth/logout - user logout
	// Responses:
	// 200 OK - user successfully logged out

	// Note: Since we're using stateless JWTs, logout can be handled on the client side, this will do nothing
}
