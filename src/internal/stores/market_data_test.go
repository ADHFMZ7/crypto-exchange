package stores

import (
	"context"
	"testing"
	"time"
)

func seedTrade(t *testing.T, buyOrderID, sellOrderID, takerOrderID int64, quantity, price int64, executedAt time.Time) int64 {
	t.Helper()

	var id int64
	err := testPool.QueryRow(context.Background(), `
		INSERT INTO trades (buy_order_id, sell_order_id, taker_order_id, market, quantity, price_each, executed_at)
		VALUES ($1, $2, $3, 'BTC-USD', $4, $5, $6)
		RETURNING id
	`, buyOrderID, sellOrderID, takerOrderID, quantity, price, executedAt).Scan(&id)
	if err != nil {
		t.Fatalf("seed trade: %v", err)
	}
	return id
}

// A user sees the executions behind their own orders, labelled with the side
// they were on rather than the side that crossed.
func TestFillsByUserIDReportsTheCallersSide(t *testing.T) {
	store := newTestStore(t)

	buyer := seedUser(t, "buyer@test")
	seller := seedUser(t, "seller@test")
	buyOrder := seedOrder(t, buyer, "buy", oneBTC, fiftyDollars, 0)
	sellOrder := seedOrder(t, seller, "sell", oneBTC, fiftyDollars, 0)
	seedTrade(t, buyOrder, sellOrder, buyOrder, oneBTC, fiftyDollars, time.Now())

	fills, err := store.FillsByUserID(context.Background(), buyer, 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(fills.Trades) != 1 {
		t.Fatalf("buyer has %d fills, want 1", len(fills.Trades))
	}

	fill := fills.Trades[0]
	if fill.Side != "buy" || fill.OrderID != buyOrder {
		t.Errorf("fill = %s on order %d, want buy on %d", fill.Side, fill.OrderID, buyOrder)
	}
	if !fill.Taker {
		t.Error("buyer crossed the spread, so their fill should be marked taker")
	}

	sellerFills, err := store.FillsByUserID(context.Background(), seller, 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(sellerFills.Trades) != 1 || sellerFills.Trades[0].Side != "sell" {
		t.Fatalf("seller fills = %+v, want one sell", sellerFills.Trades)
	}
	if sellerFills.Trades[0].Taker {
		t.Error("the seller was resting, so their fill should not be marked taker")
	}
}

// Nothing prevents a user from trading with themselves. That is two positions
// sharing one trade id, and both belong in their record.
func TestFillsByUserIDReportsBothSidesOfASelfTrade(t *testing.T) {
	store := newTestStore(t)

	user := seedUser(t, "solo@test")
	buyOrder := seedOrder(t, user, "buy", oneBTC, fiftyDollars, 0)
	sellOrder := seedOrder(t, user, "sell", oneBTC, fiftyDollars, 0)
	seedTrade(t, buyOrder, sellOrder, buyOrder, oneBTC, fiftyDollars, time.Now())

	fills, err := store.FillsByUserID(context.Background(), user, 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(fills.Trades) != 2 {
		t.Fatalf("got %d fills, want 2 — one per side", len(fills.Trades))
	}

	sides := map[string]bool{}
	for _, f := range fills.Trades {
		sides[f.Side] = true
	}
	if !sides["buy"] || !sides["sell"] {
		t.Fatalf("sides = %v, want both buy and sell", sides)
	}
}

func TestFillsByUserIDIsEmptyNotNilForANewAccount(t *testing.T) {
	store := newTestStore(t)

	fills, err := store.FillsByUserID(context.Background(), seedUser(t, "new@test"), 50)
	if err != nil {
		t.Fatal(err)
	}
	if fills.Trades == nil {
		t.Fatal("fills must encode as [] rather than null")
	}
}

func TestRecentByMarketIsNewestFirstAndNamesTheAggressor(t *testing.T) {
	store := newTestStore(t)

	buyer := seedUser(t, "buyer@test")
	seller := seedUser(t, "seller@test")
	buyOrder := seedOrder(t, buyer, "buy", oneBTC*3, fiftyDollars, 0)
	sellOrder := seedOrder(t, seller, "sell", oneBTC*3, fiftyDollars, 0)

	now := time.Now()
	seedTrade(t, buyOrder, sellOrder, sellOrder, oneBTC, 4_000, now.Add(-2*time.Minute))
	seedTrade(t, buyOrder, sellOrder, buyOrder, oneBTC, 6_000, now)

	trades, err := store.RecentByMarket(context.Background(), "BTC-USD", 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(trades.Trades) != 2 {
		t.Fatalf("got %d trades, want 2", len(trades.Trades))
	}
	if trades.Trades[0].Price != 6_000 {
		t.Errorf("first trade price = %d, want the newest (6000)", trades.Trades[0].Price)
	}
	if trades.Trades[0].TakerSide != "buy" {
		t.Errorf("taker side = %q, want buy", trades.Trades[0].TakerSide)
	}
	if trades.Trades[1].TakerSide != "sell" {
		t.Errorf("older taker side = %q, want sell", trades.Trades[1].TakerSide)
	}
}

func TestTickerOfAMarketThatHasNeverTraded(t *testing.T) {
	store := newTestStore(t)

	ticker, err := store.TickerByMarket(context.Background(), "BTC-USD", 24)
	if err != nil {
		t.Fatal(err)
	}
	if ticker.HasTraded {
		t.Fatal("HasTraded is true for a market with no trades")
	}
	if ticker.LastPrice != 0 || ticker.TradeCount != 0 || ticker.BaseVolume != 0 {
		t.Fatalf("ticker = %+v, want zeroes throughout", ticker)
	}
	if ticker.LastTradeAt != nil {
		t.Fatal("LastTradeAt should be nil when nothing has traded")
	}
}

// Change is measured against the last price before the window, which is what
// makes it a 24-hour change rather than a change since the window's first trade.
func TestTickerMeasuresChangeFromBeforeTheWindow(t *testing.T) {
	store := newTestStore(t)

	buyer := seedUser(t, "buyer@test")
	seller := seedUser(t, "seller@test")
	buyOrder := seedOrder(t, buyer, "buy", oneBTC*10, fiftyDollars, 0)
	sellOrder := seedOrder(t, seller, "sell", oneBTC*10, fiftyDollars, 0)

	now := time.Now()
	seedTrade(t, buyOrder, sellOrder, buyOrder, oneBTC, 4_000, now.Add(-48*time.Hour)) // before
	seedTrade(t, buyOrder, sellOrder, buyOrder, oneBTC, 7_000, now.Add(-2*time.Hour))  // in window
	seedTrade(t, buyOrder, sellOrder, buyOrder, oneBTC, 5_000, now.Add(-1*time.Hour))  // in window

	ticker, err := store.TickerByMarket(context.Background(), "BTC-USD", 24)
	if err != nil {
		t.Fatal(err)
	}

	if !ticker.HasTraded {
		t.Fatal("HasTraded is false despite three trades")
	}
	if ticker.LastPrice != 5_000 {
		t.Errorf("last = %d, want 5000", ticker.LastPrice)
	}
	if ticker.OpenPrice != 4_000 {
		t.Errorf("open = %d, want the 4000 from before the window", ticker.OpenPrice)
	}
	if ticker.Change != 1_000 {
		t.Errorf("change = %d, want 1000", ticker.Change)
	}
	if ticker.High != 7_000 || ticker.Low != 5_000 {
		t.Errorf("high/low = %d/%d, want 7000/5000 — the window only", ticker.High, ticker.Low)
	}
	// The 48-hour-old trade is outside the window and must not be counted.
	if ticker.TradeCount != 2 || ticker.BaseVolume != oneBTC*2 {
		t.Errorf("count/volume = %d/%d, want 2/%d", ticker.TradeCount, ticker.BaseVolume, oneBTC*2)
	}
}

// A market younger than the window has no prior price. Falling back to the
// first trade inside it keeps change meaningful instead of reporting the whole
// price as a gain from zero.
func TestTickerFallsBackToTheFirstTradeInTheWindow(t *testing.T) {
	store := newTestStore(t)

	buyer := seedUser(t, "buyer@test")
	seller := seedUser(t, "seller@test")
	buyOrder := seedOrder(t, buyer, "buy", oneBTC*10, fiftyDollars, 0)
	sellOrder := seedOrder(t, seller, "sell", oneBTC*10, fiftyDollars, 0)

	now := time.Now()
	seedTrade(t, buyOrder, sellOrder, buyOrder, oneBTC, 3_000, now.Add(-3*time.Hour))
	seedTrade(t, buyOrder, sellOrder, buyOrder, oneBTC, 3_600, now.Add(-1*time.Hour))

	ticker, err := store.TickerByMarket(context.Background(), "BTC-USD", 24)
	if err != nil {
		t.Fatal(err)
	}
	if ticker.OpenPrice != 3_000 || ticker.Change != 600 {
		t.Fatalf("open/change = %d/%d, want 3000/600", ticker.OpenPrice, ticker.Change)
	}
}
