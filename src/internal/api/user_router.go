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

// TODO: Take in user service or all services? Make decision later
type UserRouter struct {
	Services *services.Services
}

func NewUserRouter(service *services.Services) *UserRouter {
	return &UserRouter{Services: service}
}

func (router *UserRouter) Register(mux *http.ServeMux) {

	mux.Handle(
		"OPTIONS /users/",
		http.HandlerFunc(emptyHandler),
	)
	// TODO: Move this endpoint to auth router
	mux.Handle(
		"POST /users",
		http.HandlerFunc(router.UserRegister),
	)
	// TODO: Make this protected. Only user themselves or admin can access
	mux.Handle(
		"GET /users/{id}",
		http.HandlerFunc(router.UserGetHandler),
	)
	mux.Handle(
		"GET /users/me",
		auth.AuthMiddleware(http.HandlerFunc(router.UserGetSelf)),
	)
}

// Handlers below here
func (router *UserRouter) UserRegister(w http.ResponseWriter, r *http.Request) {
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

func (router *UserRouter) UserGetSelf(w http.ResponseWriter, r *http.Request) {
	// GET /users/me - get info about the authenticated user
	// Responses:
	// 200 OK - user info returned
	// 401 Unauthorized - user not authenticated

	fmt.Println("Getting self user info")

	userID := int64(r.Context().Value(auth.CtxUserKey).(int))

	user, err := router.Services.Users.GetUserByID(r.Context(), userID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(user)
}
