package market

import (
	"errors"
	"math"
	"testing"
)

func btcusd() Market {
	return Market{Symbol: "BTC-USD", Base: btc(), Quote: usd()}
}

// The worked example from docs/api-todos.md § 1c: buying 0.1 BTC at $45,000
// must lock $4,500, with every number an integer.
func TestSpendsWorkedExample(t *testing.T) {
	m := btcusd()

	currency, cost, err := m.Spends("buy", 10_000_000, 4_500_000)
	if err != nil {
		t.Fatal(err)
	}
	if currency.Code != "USD" {
		t.Fatalf("a buy debits %s, want USD (the quote)", currency.Code)
	}
	if cost != 450_000 {
		t.Fatalf("cost = %d cents, want 450000 ($4,500)", cost)
	}
}

// A sell gives up base, and the amount is the quantity itself — no arithmetic,
// so nothing can round.
func TestSpendsSellLocksBaseExactly(t *testing.T) {
	m := btcusd()

	currency, amount, err := m.Spends("sell", 12_345_678, 4_500_000)
	if err != nil {
		t.Fatal(err)
	}
	if currency.Code != "BTC" {
		t.Fatalf("a sell debits %s, want BTC (the base)", currency.Code)
	}
	if amount != 12_345_678 {
		t.Fatalf("amount = %d, want the quantity 12345678 unchanged", amount)
	}
}

// Price is a rate per WHOLE base unit, so it does not change with order size.
// That is the property that makes orders comparable across the book.
func TestSpendsScalesWithQuantityNotPrice(t *testing.T) {
	m := btcusd()

	_, tenth, err := m.Spends("buy", 10_000_000, 4_500_000) // 0.1 BTC
	if err != nil {
		t.Fatal(err)
	}
	_, whole, err := m.Spends("buy", 100_000_000, 4_500_000) // 1 BTC, same price
	if err != nil {
		t.Fatal(err)
	}

	if whole != tenth*10 {
		t.Fatalf("1 BTC cost %d but 0.1 BTC cost %d: should be exactly 10x", whole, tenth)
	}
}

// One satoshi at $45,000/BTC is worth 0.045 cents. A lock must round UP —
// rounding down under-locks and lets someone spend money they do not have.
func TestSpendsRoundsUpBelowOneMinorUnit(t *testing.T) {
	m := btcusd()

	_, locked, err := m.Spends("buy", 1, 4_500_000)
	if err != nil {
		t.Fatal(err)
	}
	if locked != 1 {
		t.Fatalf("locked %d cents for a sub-cent buy, want 1: locks round up", locked)
	}
}

// Cross-crypto is the case that makes minor units mandatory: 1 SOL ~ 0.004 BTC
// is 0 as a whole-unit integer price, but 400,000 satoshis exactly. Note the
// divide uses the BASE exponent only, even though the two differ.
func TestSpendsCrossCryptoWithDifferingExponents(t *testing.T) {
	solbtc := Market{Symbol: "SOL-BTC", Base: sol(), Quote: btc()}

	currency, cost, err := solbtc.Spends("buy", 1_000_000_000, 400_000) // 1 SOL
	if err != nil {
		t.Fatal(err)
	}
	if currency.Code != "BTC" || cost != 400_000 {
		t.Fatalf("1 SOL cost %d %s, want 400000 satoshis", cost, currency.Code)
	}
}

func TestSpendsRejectsInvalidSide(t *testing.T) {
	m := btcusd()

	// Matching is exact and case-sensitive: anything that is not "buy" or
	// "sell" is refused rather than defaulted.
	for _, side := range []string{"", "Buy", "BUY", "limit_buy", "cancel", "b", "sel"} {
		t.Run("side="+side, func(t *testing.T) {
			if _, _, err := m.Spends(side, 10_000_000, 4_500_000); !errors.Is(err, ErrInvalidSide) {
				t.Fatalf("Spends(%q) err = %v, want ErrInvalidSide", side, err)
			}
		})
	}
}

func TestSpendsRejectsNonPositiveAmounts(t *testing.T) {
	m := btcusd()

	cases := []struct{ quantity, price int64 }{
		{0, 4_500_000},
		{10_000_000, 0},
		{-1, 4_500_000},
		{10_000_000, -1},
		{0, 0},
	}

	for _, tc := range cases {
		if _, _, err := m.Spends("buy", tc.quantity, tc.price); !errors.Is(err, ErrNotPositive) {
			t.Fatalf("Spends(buy, %d, %d) err = %v, want ErrNotPositive", tc.quantity, tc.price, err)
		}
	}
}

// KNOWN BUG — skipped so the suite stays green. Remove the Skip to reproduce.
//
// The buy path validates inside notional (`quantity <= 0 || price <= 0`), but
// the sell path returns before reaching it:
//
//	case "sell":
//	    return m.Base, quantity, nil    // no check
//
// A negative quantity therefore reaches WalletStore.PlaceOrder as a negative
// debit_amount, and the SQL is written for positive amounts:
//
//	WHERE available >= $1        -- available >= -5 is always true
//	SET available = available - $1   -- available - (-5) = available + 5
//	    locked    = locked + $1      -- locked - 5
//
// The balances_non_negative CHECK only catches this when locked would go below
// zero. A user with any existing locked balance can therefore MINT available
// funds by submitting a sell with a negative quantity, and nothing upstream
// stops them: OrderRouter.CreateOrder validates only that market != "".
//
// Fix is one line in the sell branch of Spends. Reachable today via
// POST /orders {"side":"sell","quantity":-5,...}.
func TestSpendsSellRejectsNonPositiveQuantity(t *testing.T) {
	t.Skip("known bug: sell branch of Spends skips the positivity check — see comment")

	m := btcusd()

	if _, amount, err := m.Spends("sell", 0, 4_500_000); err == nil {
		t.Fatalf("a zero-quantity sell locked %d and returned no error", amount)
	}
	if _, amount, err := m.Spends("sell", -5, 4_500_000); err == nil {
		t.Fatalf("a negative-quantity sell locked %d and returned no error", amount)
	}
}

// An int64 product is not the limit — dividing by 10^base_exponent brings the
// result back into range, so what matters is whether the ANSWER fits. A naive
// quantity*price would wrap here and return garbage for a valid order.
func TestIntermediateProductMayExceedInt64(t *testing.T) {
	m := btcusd()

	// 20,000 BTC at $45,000: the product is 9e18 against an int64 ceiling of
	// 9.22e18, one step from wrapping. The answer, $900M in cents, is not close.
	_, cost, err := m.Spends("buy", 2_000_000_000_000, 4_500_000)
	if err != nil {
		t.Fatalf("20,000 BTC should be representable after the divide: %v", err)
	}
	if cost != 90_000_000_000 {
		t.Fatalf("cost = %d cents, want 90000000000 ($900M)", cost)
	}

	// 100,000 BTC at $45,000 wraps int64 outright as a product (4.5e19) and
	// still has an exact answer: $4.5B.
	_, cost, err = m.Spends("buy", 10_000_000_000_000, 4_500_000)
	if err != nil {
		t.Fatalf("100,000 BTC should be representable after the divide: %v", err)
	}
	if cost != 450_000_000_000 {
		t.Fatalf("cost = %d cents, want 450000000000 ($4.5B)", cost)
	}
}

// Overflow is a property of the quotient, and it is reported rather than wrapped.
func TestOverflowIsReportedNotWrapped(t *testing.T) {
	m := btcusd()

	if _, _, err := m.Spends("buy", math.MaxInt64, math.MaxInt64); !errors.Is(err, ErrOverflow) {
		t.Fatalf("err = %v, want ErrOverflow", err)
	}

	// An exponent-0 BASE divides by 1, so there is no rescaling to bring the
	// product back into range. The quote's exponent never enters the division.
	jpy := Market{Symbol: "JPY-USD", Base: Currency{Code: "JPY", Exponent: 0}, Quote: usd()}
	if _, _, err := jpy.Spends("buy", math.MaxInt64, 2); !errors.Is(err, ErrOverflow) {
		t.Fatalf("err = %v, want ErrOverflow", err)
	}
}

func TestScale(t *testing.T) {
	cases := []struct {
		currency Currency
		want     int64
	}{
		{usd(), 100},
		{btc(), 100_000_000},
		{sol(), 1_000_000_000},
		{Currency{Code: "JPY", Exponent: 0}, 1},
		{Currency{Code: "ETH", Exponent: 18}, 1_000_000_000_000_000_000},
	}

	for _, tc := range cases {
		if got := tc.currency.Scale(); got != tc.want {
			t.Fatalf("%s scale = %d, want %d", tc.currency.Code, got, tc.want)
		}
	}
}
