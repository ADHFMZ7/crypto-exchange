package api

import (
	"errors"
	"net/http"

	"github.com/ADHFMZ7/crypto-exchange/internal/orderbook"
	"github.com/ADHFMZ7/crypto-exchange/internal/services"
)

// MarketRouter serves the public view of a market: what it last traded at, what
// has traded recently, and what is resting on the book.
//
// None of it is authenticated. Every response is aggregate or anonymous — the
// tape carries no order ids and depth carries no owners — so there is nothing
// here that belongs to a particular account.
type MarketRouter struct {
	Services *services.Services
}

func NewMarketRouter(service *services.Services) *MarketRouter {
	return &MarketRouter{Services: service}
}

// DefaultDepthLevels is how many price levels a side are returned when the
// caller does not say. Enough to draw a book, short of streaming the whole one.
const DefaultDepthLevels = 20

func (router *MarketRouter) Register(mux *http.ServeMux) {
	mux.Handle("OPTIONS /markets/", http.HandlerFunc(emptyHandler))
	mux.Handle("OPTIONS /orderbook/", http.HandlerFunc(emptyHandler))

	mux.Handle("GET /markets/tickers", http.HandlerFunc(router.GetTickers))
	mux.Handle("GET /markets/{symbol}/ticker", http.HandlerFunc(router.GetTicker))
	mux.Handle("GET /markets/{symbol}/trades", http.HandlerFunc(router.GetMarketTrades))
	mux.Handle("GET /orderbook/{symbol}", http.HandlerFunc(router.GetOrderbook))
}

// GetTickers summarises every listed market at once.
//
// The quote board wants all of them, and asking for each in turn would be one
// round trip per market to render one table.
func (router *MarketRouter) GetTickers(w http.ResponseWriter, r *http.Request) {
	tickers, err := router.Services.Trades.Tickers(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load tickers")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"tickers": tickers})
}

func (router *MarketRouter) GetTicker(w http.ResponseWriter, r *http.Request) {
	ticker, err := router.Services.Trades.Ticker(r.Context(), r.PathValue("symbol"))
	if errors.Is(err, services.ErrUnknownMarket) {
		writeError(w, http.StatusNotFound, "unknown market")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load ticker")
		return
	}
	writeJSON(w, http.StatusOK, ticker)
}

func (router *MarketRouter) GetMarketTrades(w http.ResponseWriter, r *http.Request) {
	trades, err := router.Services.Trades.RecentTrades(r.Context(), r.PathValue("symbol"), limitParam(r))
	if errors.Is(err, services.ErrUnknownMarket) {
		writeError(w, http.StatusNotFound, "unknown market")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load trades")
		return
	}
	writeJSON(w, http.StatusOK, trades)
}

// GetOrderbook serves resting depth for one market.
//
// The book is in memory and owned by that market's worker goroutine, so this
// read is answered by the worker rather than by touching the book here. It is a
// snapshot: correct as of one instant between two requests, and stale the
// moment the next order arrives.
func (router *MarketRouter) GetOrderbook(w http.ResponseWriter, r *http.Request) {

	levels := limitParam(r)
	if levels <= 0 {
		levels = DefaultDepthLevels
	}

	snapshot, err := router.Services.Orders.Depth(r.Context(), r.PathValue("symbol"), levels)
	if errors.Is(err, services.ErrUnknownMarket) {
		writeError(w, http.StatusNotFound, "unknown market")
		return
	}
	if err != nil {
		// The only other way out is a cancelled request context, which means
		// the client is already gone.
		writeError(w, http.StatusServiceUnavailable, "order book unavailable")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"market": snapshot.Market,
		"bids":   depthDTO(snapshot.Bids),
		"asks":   depthDTO(snapshot.Asks),
	})
}

// depthLevel is the wire shape of one rung. The orderbook's own type has no
// JSON tags on purpose — it is an engine type, and the wire format is this
// package's business.
type depthLevel struct {
	Price    int64 `json:"price"`    // quote minor units per whole base
	Quantity int64 `json:"quantity"` // base minor units resting at that price
	Orders   int   `json:"orders"`
}

func depthDTO(levels []orderbook.DepthLevel) []depthLevel {
	out := make([]depthLevel, 0, len(levels))
	for _, level := range levels {
		out = append(out, depthLevel{
			Price:    int64(level.Price),
			Quantity: int64(level.Shares),
			Orders:   level.Orders,
		})
	}
	return out
}
