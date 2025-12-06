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
	Fname    string `json:"first_name"`
	Lname    string `json:"last_name"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

type Order struct {
	UserID    int     `json:"user_id"`
	Quantity  float64 `json:"quantity"`
	PriceEach float64 `json:"price_each"`
	Side      string  `json:"side"`   // buy or sell
	Market    string  `json:"market"` // e.g. BTC-USD
	Status    string  `json:"status"` // open, filled, cancelled
}
