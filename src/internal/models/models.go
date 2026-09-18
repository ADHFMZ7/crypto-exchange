package models

import "time"

// create table users (
//   id serial primary key
//   fname text,
//   lname text,

//   email text,
//   hashed_password text
//   created_at timestamptz default now()
// )

type User struct {
	ID       int64  `json:"id"`
	Fullname string `json:"fullname"`
	Email    string `json:"email"`
}

type UserAuth struct {
	ID       int64
	Fullname string
	Email    string
	Password string
}

// BalanceChange is the PATCH /wallets/me body: a signed delta applied to one
// currency. Amount is that currency's minor units — cents for USD, satoshis for
// BTC — so the deposit and the balance it lands in are denominated alike.
type BalanceChange struct {
	Currency string `json:"currency"`
	Amount   int64  `json:"amount"`
}

type Balance struct {
	ID        int64  `json:"id"`
	UserID    int64  `json:"user_id"`
	Currency  string `json:"currency"` // e.g. USD, BTC
	Available int64  `json:"available"`
	Locked    int64  `json:"locked"`
}

// type Balance struct {
// 	ID        int64     `json:"id"`
// 	UserID    int64     `json:"user_id"`
// 	Currency  *Currency `json:"currency"` // e.g. USD, BTC
// 	Available int64     `json:"available"`
// 	Locked    int64     `json:"locked"`
// }

type Wallet struct {
	UserID   int64     `json:"user_id"`
	Balances []Balance `json:"balances"`
}

type Orders struct {
	Orders []Order `json:"orders"`
}

// Order status values, mirroring the orders_status_valid CHECK added in
// migration 000003. Anything not in that constraint fails on insert, which is
// the intended direction of enforcement.
const (
	OrderOpen            = "open"
	OrderPartiallyFilled = "partially_filled"
	OrderFilled          = "filled"
	OrderCancelled       = "cancelled"
)

// Order is one limit order.
//
// Every amount is an integer count of minor units. Quantity and FilledQuantity
// are BASE minor units (satoshis on BTC-USD); PriceEach is QUOTE minor units
// per ONE WHOLE base unit (cents per whole BTC). None of them are floats —
// float64 cannot hold integers above 2^53 exactly, which is the error class
// minor units exist to remove.
//
// UserID is deliberately absent: this is only ever returned to the user whose
// orders these are, so repeating their id on every row says nothing.
type Order struct {
	ID     int64  `json:"id"`     // what DELETE /orders/{id} takes
	Market string `json:"market"` // symbol, e.g. BTC-USD

	Side           string `json:"side"`            // buy or sell
	Quantity       int64  `json:"quantity"`        // base minor units
	FilledQuantity int64  `json:"filled_quantity"` // base minor units, 0..Quantity
	PriceEach      int64  `json:"price_each"`      // quote minor units per whole base

	Status    string    `json:"status"`     // see the Order* constants above
	CreatedAt time.Time `json:"created_at"` // marshals to RFC3339
}

// Trade is a single matched fill. Each order can have several
type Trade struct {
	// ID              int64 `json:"id"`
	Market          string `json:"market"`
	RestingOrderID  int64  `json:"resting_id"`
	IncomingOrderID int64  `json:"incoming_id"`
	IncomingSide    string `json:"incoming_side"`
	Quantity        int64  `json:"quantity"`
	Price           int64  `json:"price"`

	ExecutionTime time.Time `json:"execution_time"`
}

// OrderCancel is an order the book has stopped matching, whose unspent lock the
// ledger still has to return.
type OrderCancel struct {
	Market  string `json:"market"`
	OrderID int64  `json:"order_id"`
	Side    string `json:"side"`
}

// LedgerEvent is one thing the book has done that the ledger must record.
// Exactly one field is set.
//
// Fills and cancellations share a channel because their relative order matters.
// A cancellation that overtook a fill of the same order would release a lock the
// fill still needs, and the fill would then fail against
// orders_locked_remaining_non_negative — the book would have traded and the
// ledger would not agree. One queue, applied in the order the book produced
// them, makes that unrepresentable.
type LedgerEvent struct {
	Fill   *Trade
	Cancel *OrderCancel
}

// Fill is one execution, seen from the side of it that a particular user owned.
//
// A user who was on both sides of a trade — nothing forbids it — gets one Fill
// per side, because they are two different positions that happen to share a
// trade id.
type Fill struct {
	ID      int64  `json:"id"` // the trade id; not unique across sides
	Market  string `json:"market"`
	OrderID int64  `json:"order_id"` // the caller's order, not the counterparty's

	Side     string `json:"side"`     // the caller's side: buy or sell
	Quantity int64  `json:"quantity"` // base minor units
	Price    int64  `json:"price"`    // quote minor units per whole base
	Taker    bool   `json:"taker"`    // whether the caller's order crossed the spread

	ExecutedAt time.Time `json:"executed_at"`
}

// Fills wraps the list so pagination can be added without breaking the shape,
// matching Orders.
type Fills struct {
	Trades []Fill `json:"trades"`
}

// MarketTrade is one execution as the public tape sees it: no order ids, and no
// side attributable to a person. TakerSide says which way the aggressor went,
// which is what a tape is read for.
type MarketTrade struct {
	ID         int64     `json:"id"`
	Market     string    `json:"market"`
	Quantity   int64     `json:"quantity"`
	Price      int64     `json:"price"`
	TakerSide  string    `json:"taker_side"`
	ExecutedAt time.Time `json:"executed_at"`
}

type MarketTrades struct {
	Trades []MarketTrade `json:"trades"`
}

// Ticker summarises one market over a trailing window.
//
// Prices are quote minor units per one WHOLE base unit, and BaseVolume is base
// minor units — the same units as everywhere else. Change is absolute rather
// than a percentage: a percentage is a ratio of two integers that only rounds
// well at display time, and OpenPrice is included so the client can form it
// without the server picking a precision.
//
// LastPrice is zero when the market has never traded, which HasTraded
// distinguishes from a market that genuinely traded at zero — impossible today,
// since trades_amounts_positive forbids it, but the flag costs nothing and
// stops a client inventing a rule.
type Ticker struct {
	Market string `json:"market"`

	HasTraded bool  `json:"has_traded"`
	LastPrice int64 `json:"last_price"`
	OpenPrice int64 `json:"open_price"`
	Change    int64 `json:"change"`
	High      int64 `json:"high"`
	Low       int64 `json:"low"`

	BaseVolume int64 `json:"base_volume"`
	TradeCount int64 `json:"trade_count"`

	WindowHours int        `json:"window_hours"`
	LastTradeAt *time.Time `json:"last_trade_at"`
}
