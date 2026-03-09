package models

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

type BalanceChange struct {
	// UserID int64
	Amount int64
}

// TODO: Use this model when implementing multi-currency support
type Currency struct {
	Code     string `json:"code"`     // e.g. USD, BTC
	Name     string `json:"name"`     // e.g. US Dollar, Bitcoin
	Exponent int    `json:"exponent"` // number of decimal places
}

type Balance struct {
	ID       int64   `json:"id"`
	UserID   int64   `json:"user_id"`
	Currency string  `json:"currency"` // e.g. USD, BTC
	Amount   float64 `json:"amount"`
}

// TODO: Switch to this balance later so we can maintain info abt it
// type Balance struct {
// 	ID       int64    `json:"id"`
// 	UserID   int64    `json:"user_id"`
// 	Currency Currency `json:"currency"` // e.g. USD, BTC
// 	Amount   float64  `json:"amount"`
// }

type Wallet struct {
	UserID   int64     `json:"user_id"`
	Balances []Balance `json:"balances"`
}

type Order struct {
	UserID    int64   `json:"user_id"`
	Quantity  float64 `json:"quantity"`
	PriceEach float64 `json:"price_each"`
	Side      string  `json:"side"`   // buy or sell
	Market    string  `json:"market"` // e.g. BTC-USD
	Status    string  `json:"status"` // open, filled, cancelled
}
