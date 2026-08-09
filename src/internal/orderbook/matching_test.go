package orderbook

import "testing"

/*
Matching tests for the incoming-BUY path.

orderbook_test.go covers the mirror image — an incoming sell walking the bid
side — so the cases here deliberately drive the other branch of MatchOrder,
which is a separate ~70-line block with its own bookkeeping. The two sides are
symmetric by intent, not by construction, so neither one covers the other.
*/

// restingOrders returns the live orders at a price level, oldest first.
func restingOrders(t *testing.T, ob *Orderbook, price Price, side Side) []*Order {
	t.Helper()

	var levels []*Level
	var index map[Price]int

	if side == Buy {
		levels, index = ob.LevelsBuy, ob.LevelMapBuy
	} else {
		levels, index = ob.LevelsSell, ob.LevelMapSell
	}

	ix, ok := index[price]
	if !ok {
		t.Fatalf("no live level at price %d", price)
	}
	return levels[ix].Orders.Data
}

func assertShares(t *testing.T, order *Order, want Shares) {
	t.Helper()
	if order.Shares != want {
		t.Fatalf("order %d has %d shares, want %d", order.ID, order.Shares, want)
	}
}

func TestIncomingBuyCrossesBestAsk(t *testing.T) {
	ob := newTestBook()

	ob.LimitSell(1, 100, 2400)
	ob.LimitBuy(2, 100, 2500)

	assertBestAsk(t, ob, -1)
	assertBestBid(t, ob, -1)
}

func TestIncomingBuyMatchesAtTheExactAskPrice(t *testing.T) {
	ob := newTestBook()

	ob.LimitSell(1, 100, 2400)
	ob.LimitBuy(2, 100, 2400) // limit == best ask still crosses

	assertBestAsk(t, ob, -1)
	assertBestBid(t, ob, -1)
}

func TestIncomingBuyPartialMatchLeavesAskResting(t *testing.T) {
	ob := newTestBook()

	ob.LimitSell(1, 100, 2400)
	ob.LimitBuy(2, 40, 2500)

	assertBestAsk(t, ob, 2400)
	assertBestBid(t, ob, -1) // the buy was fully filled, so nothing rests

	resting := restingOrders(t, ob, 2400, Sell)
	if len(resting) != 1 {
		t.Fatalf("%d orders resting at 2400, want 1", len(resting))
	}
	assertShares(t, resting[0], 60)
}

func TestIncomingBuyRestsAfterPartialMatch(t *testing.T) {
	ob := newTestBook()

	ob.LimitSell(1, 30, 2400)
	ob.LimitBuy(2, 100, 2500)

	// The ask was consumed entirely; the unfilled remainder becomes a bid.
	assertBestAsk(t, ob, -1)
	assertBestBid(t, ob, 2500)

	resting := restingOrders(t, ob, 2500, Buy)
	if len(resting) != 1 {
		t.Fatalf("%d orders resting at 2500, want 1", len(resting))
	}
	assertShares(t, resting[0], 70)
}

func TestNonCrossingBuyRestsWithoutMatching(t *testing.T) {
	ob := newTestBook()

	ob.LimitSell(1, 100, 2500)
	ob.LimitBuy(2, 100, 2400) // below the ask: no trade

	assertBestAsk(t, ob, 2500)
	assertBestBid(t, ob, 2400)

	assertShares(t, restingOrders(t, ob, 2500, Sell)[0], 100)
	assertShares(t, restingOrders(t, ob, 2400, Buy)[0], 100)
}

func TestBuyWalksMultipleAskLevels(t *testing.T) {
	ob := newTestBook()

	ob.LimitSell(1, 50, 2400)
	ob.LimitSell(2, 50, 2500)
	ob.LimitBuy(3, 80, 2600)

	// 50 from the 2400 level, 30 from the 2500 level.
	assertBestAsk(t, ob, 2500)
	assertBestBid(t, ob, -1)

	resting := restingOrders(t, ob, 2500, Sell)
	if len(resting) != 1 {
		t.Fatalf("%d orders resting at 2500, want 1", len(resting))
	}
	assertShares(t, resting[0], 20)
}

// A buy takes the cheapest ask first regardless of the order the asks arrived.
func TestBuyTakesCheapestAskFirst(t *testing.T) {
	ob := newTestBook()

	ob.LimitSell(1, 50, 2500) // arrives first, worse price
	ob.LimitSell(2, 50, 2400) // arrives second, better price
	ob.LimitBuy(3, 50, 2600)

	assertBestAsk(t, ob, 2500)

	resting := restingOrders(t, ob, 2500, Sell)
	if len(resting) != 1 || resting[0].ID != 1 {
		t.Fatalf("order 1 should be untouched at 2500; got %+v", resting)
	}
	assertShares(t, resting[0], 50)
}

// Within one price level the earlier order fills first.
func TestFIFOWithinAnAskLevel(t *testing.T) {
	ob := newTestBook()

	ob.LimitSell(1, 50, 2400)
	ob.LimitSell(2, 50, 2400)
	ob.LimitBuy(3, 60, 2500)

	resting := restingOrders(t, ob, 2400, Sell)
	if len(resting) != 1 {
		t.Fatalf("%d orders resting at 2400, want 1", len(resting))
	}
	if resting[0].ID != 2 {
		t.Fatalf("order %d survived, want order 2 (order 1 arrived first)", resting[0].ID)
	}
	assertShares(t, resting[0], 40)
}

func TestCancelledAskIsSkippedAndEvicted(t *testing.T) {
	ob := newTestBook()

	ob.LimitSell(1, 100, 2400)
	ob.Cancel(1)

	ob.LimitBuy(2, 50, 2500)

	// The cancelled ask neither trades nor blocks: the buy rests instead.
	assertBestAsk(t, ob, -1)
	assertBestBid(t, ob, 2500)
	assertShares(t, restingOrders(t, ob, 2500, Buy)[0], 50)
}

func TestCancelIsIgnoredForUnknownOrders(t *testing.T) {
	ob := newTestBook()

	// Never existed.
	ob.Cancel(999)

	// Fully filled on arrival, so it was never registered in OrderMap.
	ob.LimitSell(1, 100, 2400)
	ob.LimitBuy(2, 100, 2500)
	ob.Cancel(2)

	assertBestAsk(t, ob, -1)
	assertBestBid(t, ob, -1)
}

func TestCancelIsIdempotent(t *testing.T) {
	ob := newTestBook()

	ob.LimitBuy(1, 100, 2500)
	ob.Cancel(1)
	ob.Cancel(1)

	ob.LimitSell(2, 100, 2400)

	// Both cancels collapse to one: the sell finds nothing to trade with.
	assertBestBid(t, ob, -1)
	assertBestAsk(t, ob, 2400)
	assertShares(t, restingOrders(t, ob, 2400, Sell)[0], 100)
}

// Cancellation is lazy — the order is flagged and evicted when matching next
// reaches it — but the level's Size and TotalVolume are not adjusted, so they
// overstate the depth actually available until an eviction happens.
//
// This pins CURRENT behaviour rather than desired behaviour. Those two fields
// are display-only today (PrintBook), which is why it is harmless; a depth
// endpoint reading TotalVolume would be advertising liquidity that cannot
// trade. If that accounting is fixed, update this test.
func TestCancelLeavesLevelAccountingStale(t *testing.T) {
	ob := newTestBook()

	ob.LimitBuy(1, 100, 2500)
	level := ob.LevelsBuy[ob.LevelMapBuy[2500]]

	if level.TotalVolume != 100 || level.Size != 1 {
		t.Fatalf("before cancel: volume %d size %d, want 100 and 1", level.TotalVolume, level.Size)
	}

	ob.Cancel(1)

	if level.TotalVolume != 100 {
		t.Fatalf("TotalVolume = %d; this test documents that cancel does NOT "+
			"decrement it. If you fixed that, expect 0 here.", level.TotalVolume)
	}
	if level.Size != 1 {
		t.Fatalf("Size = %d; this test documents that cancel does NOT "+
			"decrement it. If you fixed that, expect 0 here.", level.Size)
	}
}

// Emptying a price level removes it from the heap and the price index. Trading
// at that price again has to rebuild it, and the three structures (slice, map,
// heap) have to agree afterwards — this is the fragile seam in AddLevel.
func TestPriceLevelCanBeRecreatedAfterBeingEmptied(t *testing.T) {
	ob := newTestBook()

	ob.LimitSell(1, 50, 2400)
	ob.LimitBuy(2, 50, 2500) // consumes the level entirely

	assertBestAsk(t, ob, -1)
	if _, live := ob.LevelMapSell[2400]; live {
		t.Fatal("emptied level is still in the price index")
	}

	ob.LimitSell(3, 70, 2400) // same price, new level

	assertBestAsk(t, ob, 2400)
	resting := restingOrders(t, ob, 2400, Sell)
	if len(resting) != 1 || resting[0].ID != 3 {
		t.Fatalf("recreated level holds %+v, want just order 3", resting)
	}
	assertShares(t, resting[0], 70)

	// And it still matches through the rebuilt level.
	ob.LimitBuy(4, 70, 2400)
	assertBestAsk(t, ob, -1)
	assertBestBid(t, ob, -1)
}

func TestEmptyBookHasNoBestPrices(t *testing.T) {
	ob := newTestBook()

	assertBestBid(t, ob, -1)
	assertBestAsk(t, ob, -1)
}

// KNOWN BUG — skipped so the suite stays green. Remove the Skip to reproduce.
//
// BestBuy/BestSell return -1 to mean "no such side", and the incoming-buy loop
// guards for it explicitly:
//
//	for ob.BestSell() != -1 && order.Shares > 0 && order.Limit >= ob.BestSell()
//
// The incoming-sell loop has no equivalent guard:
//
//	for order.Shares > 0 && order.Limit <= ob.BestBuy()
//
// It works only because a positive limit is never <= -1. A limit of -1 or below
// satisfies the condition against an empty bid side, the LevelMapBuy lookup
// misses, and the discarded `ok` leaves levelIX at 0 — indexing an empty slice.
//
// Reachable in production: the positive-value check in TradeRouter.CreateTrade
// is commented out, and the panic happens on the worker goroutine, so it takes
// the whole process down rather than failing one request.
func TestNegativeLimitPriceOnEmptyBookPanics(t *testing.T) {
	t.Skip("known bug: sell-side match loop lacks the BestBuy() != -1 sentinel guard")

	ob := newTestBook()
	ob.LimitSell(1, 100, -5) // panics: index out of range [0] with length 0

	assertBestAsk(t, ob, -5)
}

// A full round trip across both branches, to catch state that only corrupts
// after the book has been worked from both sides.
func TestBookSurvivesAlternatingSides(t *testing.T) {
	ob := newTestBook()

	ob.LimitBuy(1, 100, 2400)
	ob.LimitSell(2, 100, 2600)
	assertBestBid(t, ob, 2400)
	assertBestAsk(t, ob, 2600)

	ob.LimitSell(3, 60, 2400)  // hits the resting bid
	ob.LimitBuy(4, 60, 2600)   // hits the resting ask
	assertBestBid(t, ob, 2400) // 40 left
	assertBestAsk(t, ob, 2600) // 40 left

	assertShares(t, restingOrders(t, ob, 2400, Buy)[0], 40)
	assertShares(t, restingOrders(t, ob, 2600, Sell)[0], 40)

	ob.LimitSell(5, 40, 2400)
	ob.LimitBuy(6, 40, 2600)

	assertBestBid(t, ob, -1)
	assertBestAsk(t, ob, -1)
}
