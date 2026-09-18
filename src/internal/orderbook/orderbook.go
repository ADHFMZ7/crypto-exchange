package orderbook

import (
	"container/heap"
	"fmt"
	"sort"
	"time"
)

// A matching engine order book implementation

type Side bool

const (
	Sell Side = false
	Buy  Side = true
)

type OrderID int64
type Price int64
type Shares int64
type Time int64

type Level struct {
	LimitPrice  Price
	Size        int
	TotalVolume Shares
	Orders      Queue
}

type Order struct {
	ID        OrderID
	Side      Side
	Shares    Shares
	Limit     Price
	EntryTime time.Time
	EventTime time.Time

	Cancelled bool
}

// Trade is a single matched fill. Each order can have several
type Trade struct {
	RestingOrderID  OrderID
	IncomingOrderID OrderID
	IncomingSide    Side
	Quantity        Shares
	Price           Price

	ExecutionTime time.Time
}

type Orderbook struct {
	// We store levels in a slice and maintain a map to the index
	LevelsSell   []*Level
	LevelsBuy    []*Level
	LevelMapBuy  map[Price]int
	LevelMapSell map[Price]int
	prices       map[Price]int // Limit index

	// Used to find the best price fast
	LowestSell *MinHeap
	HighestBuy *MaxHeap

	Orders   []*Order
	OrderMap map[OrderID]int
}

func NewOrderbook() *Orderbook {
	var ob Orderbook

	ob.LowestSell = &MinHeap{}
	ob.HighestBuy = &MaxHeap{}

	ob.LevelMapBuy = make(map[Price]int)
	ob.LevelMapSell = make(map[Price]int)
	ob.OrderMap = make(map[OrderID]int)

	heap.Init(ob.LowestSell)
	heap.Init(ob.HighestBuy)

	return &ob
}

func (ob *Orderbook) MatchOrder(order *Order) []Trade {

	// fmt.Printf("\n[MatchOrder] Incoming order ID=%d Side=%v Shares=%d Limit=%d\n",
	// 	order.ID, order.Side, order.Shares, order.Limit)

	var trades []Trade

	if order.Side == Buy {

		// fmt.Printf("[MatchOrder] BestSell=%d\n", ob.BestSell())

		for ob.BestSell() != -1 && (order.Shares > 0) && (order.Limit >= ob.BestSell()) {

			// fmt.Printf("[MatchOrder] Buy order crosses spread: limit=%d >= bestSell=%d\n",
			// 	order.Limit, ob.BestSell())

			levelIX, ok := ob.LevelMapSell[ob.BestSell()]
			// fmt.Printf("[MatchOrder] LevelMapSell lookup price=%d -> index=%d ok=%v\n",
			// 	ob.BestSell(), levelIX, ok)

			level := ob.LevelsSell[levelIX]

			// fmt.Printf("[MatchOrder] Operating on sell level price=%d volume=%d orders=%d\n",
			// 	level.LimitPrice, level.TotalVolume, level.Size)

			sell_order, ok := level.Orders.Peek()

			for (order.Shares > 0) && ok {

				// fmt.Printf("[MatchOrder] Inspecting sell order ID=%d shares=%d cancelled=%v\n",
				// 	sell_order.ID, sell_order.Shares, sell_order.Cancelled)

				if sell_order.Cancelled == true {
					// fmt.Printf("[MatchOrder] Removing cancelled order ID=%d\n", sell_order.ID)
					level.Orders.Dequeue()
					sell_order, ok = level.Orders.Peek()
					continue
				}

				trade_qty := min(sell_order.Shares, order.Shares)
				// trade_price := Price(int64(trade_qty) * int64(sell_order.Limit))
				trade_time := time.Now()

				// fmt.Printf("[MatchOrder] Executing trade qty=%d price=%d\n",
				// 	trade_qty, sell_order.Limit)

				// fmt.Printf("[MatchOrder] Total trade price: %d\n",
				// 	trade_price)

				order.Shares -= trade_qty
				order.EventTime = trade_time

				sell_order.Shares -= trade_qty
				sell_order.EventTime = trade_time
				level.TotalVolume -= trade_qty

				// Aggressor is the buy
				trades = append(trades, Trade{
					RestingOrderID:  sell_order.ID,
					IncomingOrderID: order.ID,
					IncomingSide:    Buy,
					Quantity:        trade_qty,
					Price:           sell_order.Limit,
					ExecutionTime:   trade_time,
				})

				// fmt.Printf("[MatchOrder] Post-trade buyerShares=%d sellerShares=%d\n",
				// 	order.Shares, sell_order.Shares)

				if sell_order.Shares <= 0 {
					// fmt.Printf("[MatchOrder] Sell order fully filled ID=%d\n",
					// 	sell_order.ID)
					level.Orders.Dequeue()
					level.Size -= 1
				}

				sell_order, ok = level.Orders.Peek()
			}
			if !ok {
				// fmt.Println("[MatchOrder] Level empty, removing level")

				heap.Pop(ob.LowestSell)
				delete(ob.LevelMapSell, level.LimitPrice)

				continue
			}

			// fmt.Println("[MatchOrder] Finished processing current price level")

		}

	} else if order.Side == Sell {

		for ob.BestBuy() != -1 && (order.Shares > 0) && (order.Limit <= ob.BestBuy()) {

			// fmt.Printf("[MatchOrder] Sell order crosses spread: limit=%d >= bestSell=%d\n",
			// 	order.Limit, ob.BestBuy())

			levelIX, ok := ob.LevelMapBuy[ob.BestBuy()]
			// fmt.Printf("[MatchOrder] LevelMapBuy lookup price=%d -> index=%d ok=%v\n",
			// 	ob.BestBuy(), levelIX, ok)

			level := ob.LevelsBuy[levelIX]

			// fmt.Printf("[MatchOrder] Operating on Buy level price=%d volume=%d orders=%d\n",
			// 	level.LimitPrice, level.TotalVolume, level.Size)

			buy_order, ok := level.Orders.Peek()

			for (order.Shares > 0) && ok {

				// fmt.Printf("[MatchOrder] Inspecting Buy order ID=%d shares=%d cancelled=%v\n",
				// 	buy_order.ID, buy_order.Shares, buy_order.Cancelled)

				if buy_order.Cancelled == true {
					// fmt.Printf("[MatchOrder] Removing cancelled order ID=%d\n", buy_order.ID)
					level.Orders.Dequeue()
					buy_order, ok = level.Orders.Peek()
					continue
				}

				trade_qty := min(buy_order.Shares, order.Shares)
				// trade_price := Price(int64(trade_qty) * int64(buy_order.Limit))
				trade_time := time.Now()

				// fmt.Printf("[MatchOrder] Executing trade qty=%d price=%d\n",
				// 	trade_qty, buy_order.Limit)

				// fmt.Printf("[MatchOrder] Total trade price: %d\n",
				// 	trade_price)

				order.Shares -= trade_qty
				order.EventTime = trade_time

				buy_order.Shares -= trade_qty
				buy_order.EventTime = trade_time
				level.TotalVolume -= trade_qty

				// Aggressor is the sell
				trades = append(trades, Trade{
					RestingOrderID:  buy_order.ID,
					IncomingOrderID: order.ID,
					IncomingSide:    Sell,
					Quantity:        trade_qty,
					Price:           buy_order.Limit,
					ExecutionTime:   trade_time,
				})

				// fmt.Printf("[MatchOrder] Post-trade sellerShares=%d buyerShares=%d\n",
				// 	order.Shares, buy_order.Shares)

				if buy_order.Shares <= 0 {
					// fmt.Printf("[MatchOrder] Sell order fully filled ID=%d\n",
					// 	buy_order.ID)
					level.Orders.Dequeue()
					level.Size -= 1
				}

				buy_order, ok = level.Orders.Peek()
			}
			if !ok {
				// fmt.Println("[MatchOrder] Level empty, removing level")

				heap.Pop(ob.HighestBuy)
				delete(ob.LevelMapBuy, level.LimitPrice)

				continue
			}

			// fmt.Println("[MatchOrder] Finished processing current price level")

		}

	} else {

		// fmt.Println("[MatchOrder] No matching possible (book empty or no price cross)")
	}

	// fmt.Println("[MatchOrder] Matching finished")

	return trades
}

func (ob *Orderbook) LimitSell(orderId OrderID, shares Shares, limit Price) []Trade {

	// fmt.Printf("\n[LimitSell] New SELL order ID=%d Shares=%d Limit=%d\n",
	// orderId, shares, limit)

	entry_time := time.Now()

	order := Order{
		ID:        orderId,
		Side:      Sell,
		Shares:    shares,
		EntryTime: entry_time,
		EventTime: entry_time,
		Limit:     limit,
		Cancelled: false,
	}

	// fmt.Println("[LimitSell] Attempting to match order")

	trades := ob.MatchOrder(&order)

	// fmt.Printf("[LimitSell] Remaining shares after matching=%d\n", order.Shares)

	if order.Shares > 0 {
		// fmt.Println("[LimitSell] Adding remaining shares to book")
		ob.AddOrder(&order)
	}

	return trades
}

func (ob *Orderbook) LimitBuy(orderId OrderID, shares Shares, limit Price) []Trade {

	// fmt.Printf("\n[LimitBuy] New BUY order ID=%d Shares=%d Limit=%d\n",
	// 	orderId, shares, limit)

	entry_time := time.Now()

	order := Order{
		ID:        orderId,
		Side:      Buy,
		Shares:    shares,
		EntryTime: entry_time,
		EventTime: entry_time,
		Limit:     limit,
		Cancelled: false,
	}

	// fmt.Println("[LimitBuy] Attempting to match order")

	trades := ob.MatchOrder(&order)

	// fmt.Printf("[LimitBuy] Remaining shares after matching=%d\n", order.Shares)

	if order.Shares > 0 {
		// fmt.Println("[LimitBuy] Adding remaining shares to book")
		ob.AddOrder(&order)
	}

	return trades
}

func (ob *Orderbook) Cancel(orderId OrderID) {
	// Lazily marks order as cancelled. Order later evicted on execute

	orderIx, ok := ob.OrderMap[orderId]
	if !ok {
		// Error: Order does not exist. Figure out what to do here
		return
	}

	order := ob.Orders[orderIx]
	order.Cancelled = true
}

// BestSell is the lowest resting ask, or -1 when there are none.
//
// -1 is a sentinel rather than an error, so every caller must test for it
// before using the result as a price. Both match loops do: without that test a
// book with nothing on one side answers -1, the level lookup misses, and
// indexing the empty level slice panics — which in a worker goroutine takes the
// process with it.
func (ob *Orderbook) BestSell() Price {
	// Returns the min sell price
	price, ok := ob.LowestSell.Peek()
	if !ok {
		return Price(-1)
	}
	return price
}

func (ob *Orderbook) BestBuy() Price {
	// Returns the max buy price
	price, ok := ob.HighestBuy.Peek()
	if !ok {
		return Price(-1)
	}
	return price
}

func (ob *Orderbook) AddOrder(order *Order) {

	// fmt.Printf("[AddOrder] Adding order ID=%d Side=%v Shares=%d Limit=%d\n",
	// order.ID, order.Side, order.Shares, order.Limit)

	var levelIx int
	var ok bool
	var level *Level

	if order.Side == Buy {

		// fmt.Printf("[AddOrder] Looking up BUY level for price=%d\n", order.Limit)

		levelIx, ok = ob.LevelMapBuy[order.Limit]

		if !ok {
			// fmt.Println("[AddOrder] Buy level does not exist. Creating new level")
			levelIx = ob.AddLevel(order.Limit, Buy)
		}

		level = ob.LevelsBuy[levelIx]

	} else {

		// fmt.Printf("[AddOrder] Looking up SELL level for price=%d\n", order.Limit)

		levelIx, ok = ob.LevelMapSell[order.Limit]

		if !ok {
			// fmt.Println("[AddOrder] Sell level does not exist. Creating new level")
			levelIx = ob.AddLevel(order.Limit, Sell)
		}

		level = ob.LevelsSell[levelIx]
	}

	// fmt.Printf("[AddOrder] Appending order to level price=%d\n", level.LimitPrice)

	level.Orders.Enqueue(order)

	level.Size++
	level.TotalVolume += order.Shares

	// fmt.Printf("[AddOrder] Level stats updated size=%d volume=%d\n",
	// 	level.Size, level.TotalVolume)

	orderIx := len(ob.Orders)

	ob.Orders = append(ob.Orders, order)
	ob.OrderMap[order.ID] = orderIx

	// fmt.Printf("[AddOrder] Order registered globally index=%d\n", orderIx)
	// ob.PrintBook()
}

func (ob *Orderbook) AddLevel(limitPrice Price, side Side) int {

	// fmt.Printf("[AddLevel] Creating new level price=%d side=%v\n",
	// 	limitPrice, side)

	var level Level
	level.LimitPrice = limitPrice
	level.Size = 0
	level.TotalVolume = 0

	if side == Buy {

		levelIx := len(ob.LevelsBuy)

		ob.LevelsBuy = append(ob.LevelsBuy, &level)
		ob.LevelMapBuy[limitPrice] = levelIx

		// fmt.Printf("[AddLevel] BUY level index=%d added to heap\n", levelIx)

		heap.Push(ob.HighestBuy, limitPrice)

		return levelIx

	} else {

		levelIx := len(ob.LevelsSell)

		ob.LevelsSell = append(ob.LevelsSell, &level)
		ob.LevelMapSell[limitPrice] = levelIx

		// fmt.Printf("[AddLevel] SELL level index=%d added to heap\n", levelIx)

		heap.Push(ob.LowestSell, limitPrice)

		return levelIx
	}
}

func (ob *Orderbook) PrintBook() {

	fmt.Println("\n================ ORDER BOOK ================")

	// ------------------------------------------------
	// Print SELL side (asks)
	// Best ask should appear first
	// ------------------------------------------------
	fmt.Println("\nSELL SIDE (Asks)")

	if len(ob.LevelsSell) == 0 {
		fmt.Println("  <empty>")
	}

	for _, level := range ob.LevelsSell {

		fmt.Printf("Price: %d | Orders: %d | Volume: %d\n",
			level.LimitPrice,
			level.Size,
			level.TotalVolume,
		)

		// Iterate through FIFO queue
		for _, order := range level.Orders.Data {

			if order.Cancelled {
				fmt.Printf("    Order %d | %d shares | CANCELLED\n",
					order.ID,
					order.Shares,
				)
				continue
			}

			fmt.Printf("    Order %d | %d shares\n",
				order.ID,
				order.Shares,
			)
		}
	}

	// ------------------------------------------------
	// Print BUY side (bids)
	// Best bid should appear first
	// ------------------------------------------------
	fmt.Println("\nBUY SIDE (Bids)")

	if len(ob.LevelsBuy) == 0 {
		fmt.Println("  <empty>")
	}

	for _, level := range ob.LevelsBuy {

		fmt.Printf("Price: %d | Orders: %d | Volume: %d\n",
			level.LimitPrice,
			level.Size,
			level.TotalVolume,
		)

		for _, order := range level.Orders.Data {

			if order.Cancelled {
				fmt.Printf("    Order %d | %d shares | CANCELLED\n",
					order.ID,
					order.Shares,
				)
				continue
			}

			fmt.Printf("    Order %d | %d shares\n",
				order.ID,
				order.Shares,
			)
		}
	}

	fmt.Println("\n============================================")
}

// DepthLevel is the live resting volume at one price.
type DepthLevel struct {
	Price  Price
	Shares Shares
	Orders int
}

// Depth reports resting volume per price — bids best-first (descending), asks
// best-first (ascending) — capped at limit levels a side. A limit of 0 or less
// returns every level.
//
// Volume is recounted from the orders at each level rather than read from
// Level.TotalVolume, which a cancellation leaves stale: Cancel only flags the
// order, and the level's running total is not corrected until matching reaches
// it and evicts it. Reporting TotalVolume would advertise depth that is not
// there.
//
// Levels are kept in their slices once created, even after emptying, so a level
// with nothing live behind it is dropped rather than published as a zero.
func (ob *Orderbook) Depth(limit int) (bids, asks []DepthLevel) {
	return liveLevels(ob.LevelsBuy, Buy, limit), liveLevels(ob.LevelsSell, Sell, limit)
}

func liveLevels(levels []*Level, side Side, limit int) []DepthLevel {
	out := make([]DepthLevel, 0, len(levels))

	for _, level := range levels {
		var shares Shares
		var orders int

		for _, order := range level.Orders.Data {
			if order.Cancelled || order.Shares <= 0 {
				continue
			}
			shares += order.Shares
			orders++
		}

		if orders == 0 {
			continue
		}
		out = append(out, DepthLevel{Price: level.LimitPrice, Shares: shares, Orders: orders})
	}

	// Best first: the highest bid and the lowest ask are the top of each book.
	sort.Slice(out, func(i, j int) bool {
		if side == Buy {
			return out[i].Price > out[j].Price
		}
		return out[i].Price < out[j].Price
	})

	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}
