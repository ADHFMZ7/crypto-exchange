package orderbook

import "testing"

func TestDepthAggregatesOrdersAtEachPrice(t *testing.T) {
	ob := newTestBook()

	ob.LimitBuy(1, 100, 2400)
	ob.LimitBuy(2, 50, 2400) // same level as order 1
	ob.LimitBuy(3, 70, 2300)
	ob.LimitSell(4, 30, 2600)

	bids, asks := ob.Depth(0)

	if len(bids) != 2 {
		t.Fatalf("got %d bid levels, want 2", len(bids))
	}
	// Best bid first.
	if bids[0].Price != 2400 || bids[0].Shares != 150 || bids[0].Orders != 2 {
		t.Errorf("best bid = %+v, want price 2400, 150 shares, 2 orders", bids[0])
	}
	if bids[1].Price != 2300 || bids[1].Shares != 70 {
		t.Errorf("second bid = %+v, want price 2300, 70 shares", bids[1])
	}
	if len(asks) != 1 || asks[0].Price != 2600 || asks[0].Shares != 30 {
		t.Errorf("asks = %+v, want one level of 30 at 2600", asks)
	}
}

// Asks ascend, bids descend: both are "best first", which is the order a book
// is read in.
func TestDepthOrdersEachSideBestFirst(t *testing.T) {
	ob := newTestBook()

	ob.LimitSell(1, 10, 2800)
	ob.LimitSell(2, 10, 2600)
	ob.LimitSell(3, 10, 2700)

	_, asks := ob.Depth(0)

	for i, want := range []Price{2600, 2700, 2800} {
		if asks[i].Price != want {
			t.Fatalf("ask level %d = %d, want %d", i, asks[i].Price, want)
		}
	}
}

// Cancel only flags an order, and the level's TotalVolume is not corrected
// until matching evicts it. Depth must not advertise that stale volume.
func TestDepthExcludesCancelledOrders(t *testing.T) {
	ob := newTestBook()

	ob.LimitBuy(1, 100, 2400)
	ob.LimitBuy(2, 50, 2400)
	ob.Cancel(1)

	bids, _ := ob.Depth(0)

	if len(bids) != 1 {
		t.Fatalf("got %d bid levels, want 1", len(bids))
	}
	if bids[0].Shares != 50 || bids[0].Orders != 1 {
		t.Fatalf("bid = %+v, want the 50 live shares only", bids[0])
	}

	level := ob.LevelsBuy[ob.LevelMapBuy[2400]]
	if level.TotalVolume != 150 {
		t.Fatalf("precondition failed: TotalVolume = %d, expected it to still read 150 stale",
			level.TotalVolume)
	}
}

// A level that empties is kept in the slice so its index stays valid. It must
// not surface as a zero-volume rung.
func TestDepthDropsEmptiedLevels(t *testing.T) {
	ob := newTestBook()

	ob.LimitSell(1, 100, 2400)
	ob.LimitBuy(2, 100, 2400) // consumes the level entirely

	bids, asks := ob.Depth(0)
	if len(bids) != 0 || len(asks) != 0 {
		t.Fatalf("depth = %+v / %+v, want both empty", bids, asks)
	}
}

func TestDepthRespectsTheLevelLimit(t *testing.T) {
	ob := newTestBook()

	for i, price := range []Price{2400, 2300, 2200, 2100} {
		ob.LimitBuy(OrderID(i+1), 10, price)
	}

	bids, _ := ob.Depth(2)

	if len(bids) != 2 {
		t.Fatalf("got %d bid levels, want 2", len(bids))
	}
	// The two best, not an arbitrary two.
	if bids[0].Price != 2400 || bids[1].Price != 2300 {
		t.Fatalf("bids = %+v, want 2400 then 2300", bids)
	}
}

func TestDepthOfAnEmptyBookIsEmptyNotNil(t *testing.T) {
	bids, asks := newTestBook().Depth(0)

	// Non-nil so the JSON encodes as [] rather than null.
	if bids == nil || asks == nil {
		t.Fatal("depth of an empty book must be empty slices, not nil")
	}
	if len(bids) != 0 || len(asks) != 0 {
		t.Fatalf("depth = %+v / %+v, want empty", bids, asks)
	}
}
