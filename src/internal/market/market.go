package market

import (
	"errors"
	"math/big"
)

type Market struct {
	Symbol string
	Quote  Currency
	Base   Currency
}

var (
	ErrNotPositive = errors.New("quantity and price must be positive")
	ErrOverflow    = errors.New("order notional overflows int64")
	ErrInvalidSide = errors.New("Invalid Side")
)

type rounding int

const (
	roundDown rounding = iota
	roundUp
)

func (m *Market) notional(quantity, price int64, mode rounding) (int64, error) {
	if quantity <= 0 || price <= 0 {
		return 0, ErrNotPositive
	}

	product := new(big.Int).Mul(big.NewInt(quantity), big.NewInt(price))
	scale := big.NewInt(m.Base.Scale())

	quotient, remainder := new(big.Int).QuoRem(product, scale, new(big.Int))

	if mode == roundUp && remainder.Sign() != 0 {
		quotient.Add(quotient, big.NewInt(1))
	}
	if !quotient.IsInt64() {
		return 0, ErrOverflow
	}

	return quotient.Int64(), nil
}

// FillNotional reports the quote amount that changes hands for one fill.
//
// Rounded up, the same direction Spends locked at, so the amounts settlement
// releases can never add up to more than the buyer had locked.
func (m *Market) FillNotional(quantity, price int64) (int64, error) {
	return m.notional(quantity, price, roundUp)
}

// Locks reports which currency a side's funds are held in while an order rests.
//
// A buy locks quote to pay with, a sell locks the base it is selling. Releasing
// a cancelled order needs this without knowing the amount, which is why it is
// separate from Spends rather than read off it.
func (m *Market) Locks(side string) (Currency, error) {
	switch side {
	case "buy":
		return m.Quote, nil
	case "sell":
		return m.Base, nil
	}
	return Currency{}, ErrInvalidSide
}

// Spends reports the currency debited and the amount to lock.
func (m *Market) Spends(side string, quantity int64, price int64) (Currency, int64, error) {

	if quantity <= 0 || price <= 0 {
		return Currency{}, 0, ErrNotPositive
	}

	switch side {
	case "buy":
		cost, err := m.notional(quantity, price, roundUp)
		return m.Quote, cost, err
	case "sell":
		return m.Base, quantity, nil
	}
	return Currency{}, 0, ErrInvalidSide

}

// Receives reports the currency credited, and the amount on a full fill.
// func (m Market) Receives(side Side, quantity, price int64) (Currency, int64, error)
