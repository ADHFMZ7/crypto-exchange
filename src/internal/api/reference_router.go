package api

import (
	"net/http"

	"github.com/ADHFMZ7/crypto-exchange/internal/market"
	"github.com/ADHFMZ7/crypto-exchange/internal/services"
)

// TODO: Take in user service or all services? Make decision later
type ReferenceRouter struct {
	// Services *services.Services
	Registry *market.Registry
}

func NewReferenceRouter(service *services.Services) *ReferenceRouter {
	// return &ReferenceRouter{Services: service, Registry: service.Trades.MarketRegistry}
	return &ReferenceRouter{Registry: service.Orders.Registry}
}

func (router *ReferenceRouter) Register(mux *http.ServeMux) {

	mux.Handle(
		"GET /currencies",
		http.HandlerFunc(router.GetCurrencies),
	)
	mux.Handle(
		"GET /markets",
		http.HandlerFunc(router.GetMarkets),
	)
}

func (router *ReferenceRouter) GetCurrencies(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, router.Registry.Currencies())
}

func (router *ReferenceRouter) GetMarkets(w http.ResponseWriter, r *http.Request) {

	markets := router.Registry.Markets()

	type marketDTO struct {
		Symbol string `json:"symbol"`
		Base   string `json:"base"`
		Quote  string `json:"quote"`
	}

	out := make([]marketDTO, 0, len(markets))
	for _, m := range markets {
		out = append(out, marketDTO{
			Symbol: m.Symbol,
			Base:   m.Base.Code,
			Quote:  m.Quote.Code,
		})
	}

	writeJSON(w, http.StatusOK, out)
}
