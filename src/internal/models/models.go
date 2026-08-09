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
