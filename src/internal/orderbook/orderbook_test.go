package orderbook

import (
	"testing"
)

// Helper to create a fresh orderbook
func newTestBook() *Orderbook {
	ob := NewOrderbook()

	return ob
}

func assertBestBid(t *testing.T, ob *Orderbook, expected Price) {
	t.Helper()
	if ob.BestBuy() != expected {
		t.Fatalf("expected best bid %d got %d", expected, ob.BestBuy())
	}
}

func assertBestAsk(t *testing.T, ob *Orderbook, expected Price) {
	t.Helper()
	if ob.BestSell() != expected {
		t.Fatalf("expected best ask %d got %d", expected, ob.BestSell())
	}
}

func TestIncomingSellCrossesBestBid(t *testing.T) {
	ob := newTestBook()

	ob.LimitBuy(1, 100, 2500)
	ob.LimitSell(2, 100, 2400)

	if ob.BestBuy() != -1 {
		t.Fatalf("expected no remaining bids")
	}

	if ob.BestSell() != -1 {
		t.Fatalf("expected no remaining asks")
	}
}

func TestIncomingSellPartialMatch(t *testing.T) {
	ob := newTestBook()

	ob.LimitBuy(1, 100, 2500)
	ob.LimitSell(2, 40, 2400)

	assertBestBid(t, ob, 2500)

	level := ob.LevelsBuy[ob.LevelMapBuy[2500]]

	order, ok := level.Orders.Peek()
	if !ok {
		t.Fatalf("expected remaining buy order")
	}

	if order.Shares != 60 {
		t.Fatalf("expected 60 remaining shares got %d", order.Shares)
	}
}

func TestSellWalksMultipleBidLevels(t *testing.T) {
	ob := newTestBook()

	ob.LimitBuy(1, 50, 2500)
	ob.LimitBuy(2, 50, 2400)

	ob.LimitSell(3, 80, 2300)

	assertBestBid(t, ob, 2400)

	level := ob.LevelsBuy[ob.LevelMapBuy[2400]]

	order, ok := level.Orders.Peek()
	if !ok {
		t.Fatalf("expected remaining buy order")
	}

	if order.Shares != 20 {
		t.Fatalf("expected 20 remaining shares got %d", order.Shares)
	}
}

func TestPricePriority(t *testing.T) {
	ob := newTestBook()

	ob.LimitBuy(1, 50, 2400)
	ob.LimitBuy(2, 50, 2500)

	ob.LimitSell(3, 50, 2300)

	assertBestBid(t, ob, 2400)
}

func TestFIFOAfterPartialFill(t *testing.T) {
	ob := newTestBook()

	ob.LimitBuy(1, 50, 2500)
	ob.LimitBuy(2, 50, 2500)

	ob.LimitSell(3, 60, 2400)

	assertBestBid(t, ob, 2500)

	level := ob.LevelsBuy[ob.LevelMapBuy[2500]]

	order, ok := level.Orders.Peek()
	if !ok {
		t.Fatalf("expected remaining order")
	}

	if order.ID != 2 {
		t.Fatalf("expected FIFO order 2 remaining got %d", order.ID)
	}

	if order.Shares != 40 {
		t.Fatalf("expected 40 shares remaining got %d", order.Shares)
	}
}

func TestCancelledOrderSkippedDuringMatch(t *testing.T) {
	ob := newTestBook()

	ob.LimitBuy(1, 100, 2500)
	ob.Cancel(1)

	ob.LimitSell(2, 50, 2400)

	assertBestAsk(t, ob, 2400)

	level := ob.LevelsSell[ob.LevelMapSell[2400]]

	order, ok := level.Orders.Peek()
	if !ok {
		t.Fatalf("expected resting sell order")
	}

	if order.Shares != 50 {
		t.Fatalf("expected 50 shares resting got %d", order.Shares)
	}
}

func TestSellRestingAfterPartialMatch(t *testing.T) {
	ob := newTestBook()

	ob.LimitBuy(1, 50, 2500)
	ob.LimitSell(2, 100, 2400)

	assertBestAsk(t, ob, 2400)

	level := ob.LevelsSell[ob.LevelMapSell[2400]]

	order, ok := level.Orders.Peek()
	if !ok {
		t.Fatalf("expected resting sell order")
	}

	if order.Shares != 50 {
		t.Fatalf("expected 50 shares remaining got %d", order.Shares)
	}
}

func TestCrossingOrderDoesNotRest(t *testing.T) {
	ob := newTestBook()

	ob.LimitBuy(1, 100, 2500)
	ob.LimitSell(2, 100, 2500)

	if ob.BestBuy() != -1 {
		t.Fatalf("expected no remaining bids")
	}

	if ob.BestSell() != -1 {
		t.Fatalf("expected no remaining asks")
	}
}
