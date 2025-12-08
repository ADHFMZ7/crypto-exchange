package api

import (
	"net/http"

	"github.com/ADHFMZ7/crypto-exchange/internal/services"
)



func NewRouter(services *services.Services) *http.ServeMux {
	
	mux := http.NewServeMux()

	NewUserRouter(services).Register(mux)	

	return mux
}
	
