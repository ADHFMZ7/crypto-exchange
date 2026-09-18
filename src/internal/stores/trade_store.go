package stores

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/ADHFMZ7/crypto-exchange/internal/models"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type TradeStore struct {
	pool *pgxpool.Pool
}

// ErrNoBalance is returned when settlement tries to debit a balance row that
// does not exist, which means it is working from state the ledger never saw.
var ErrNoBalance = errors.New("settlement: no balance row to debit")

// Settle records a trade and moves everything it implies, in one transaction.
//
// quoteAmount is passed in rather than derived here: turning quantity and price
// into money needs the base exponent and a rounding direction, and both already
// live in market.notional. Recomputing it in SQL would put the rounding rule in
// a second place.
//
// Atomicity is the point. filled_quantity is a denormalised summary of this
// ledger (see migration 000003), so a trade row written without its order
// update leaves the two permanently disagreeing — and a debited buyer beside an
// uncredited seller is money destroyed.
func (store *TradeStore) Settle(ctx context.Context, eventID int64, trade models.Trade,
	baseCurrency, quoteCurrency string, quoteAmount int64) error {

	buyOrderID, sellOrderID := trade.RestingOrderID, trade.IncomingOrderID
	if trade.IncomingSide == "buy" {
		buyOrderID, sellOrderID = trade.IncomingOrderID, trade.RestingOrderID
	}

	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) // no-op if already committed

	// Before anything moves. A fill is not idempotent — replaying one inserts a
	// second trade row, advances filled_quantity again and moves the money
	// again — so the claim and the effect have to commit together.
	if err := claimEvent(ctx, tx, eventID); err != nil {
		return err
	}

	// Lowest id first, so two settlements can never hold what the other needs.
	firstID, secondID := buyOrderID, sellOrderID
	if secondID < firstID {
		firstID, secondID = secondID, firstID
	}
	first, err := lockOrder(ctx, tx, firstID)
	if err != nil {
		return err
	}
	second, err := lockOrder(ctx, tx, secondID)
	if err != nil {
		return err
	}

	buy, sell := first, second
	if firstID != buyOrderID {
		buy, sell = second, first
	}

	// The buyer locked at their own limit, so a fill against a cheaper resting
	// order leaves money owed back to them. Releasing exactly what each fill
	// costs, then everything left on the final fill, makes the releases sum to
	// the original lock however the intermediate rounding went — locked always
	// reaches zero, with no dust and nothing to drive it negative.
	buyRelease := quoteAmount
	if trade.Quantity >= buy.remaining {
		buyRelease = buy.locked
	}
	// Base was locked one for one with quantity, so there is nothing to round.
	sellRelease := trade.Quantity
	if trade.Quantity >= sell.remaining {
		sellRelease = sell.locked
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO trades
			(buy_order_id, sell_order_id, taker_order_id, market, quantity, price_each, executed_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, buyOrderID, sellOrderID, trade.IncomingOrderID, trade.Market,
		trade.Quantity, trade.Price, trade.ExecutionTime)
	if err != nil {
		return err
	}

	if err := fillOrder(ctx, tx, buyOrderID, trade.Quantity, buyRelease); err != nil {
		return err
	}
	if err := fillOrder(ctx, tx, sellOrderID, trade.Quantity, sellRelease); err != nil {
		return err
	}

	// Buyer gives up quote and receives base; the seller the reverse. Whatever
	// the buyer's lock released beyond the cost returns to their available, so
	// the four moves sum to zero.
	moves := []struct {
		userID    int64
		currency  string
		available int64
		locked    int64
	}{
		{buy.userID, quoteCurrency, buyRelease - quoteAmount, -buyRelease},
		{buy.userID, baseCurrency, trade.Quantity, 0},
		{sell.userID, baseCurrency, 0, -sellRelease},
		{sell.userID, quoteCurrency, quoteAmount, 0},
	}
	for _, m := range moves {
		if err := moveBalance(ctx, tx, m.userID, m.currency, m.available, m.locked); err != nil {
			return err
		}
	}

	return tx.Commit(ctx)
}

// orderLock is the settlement-relevant state of one order, read under a lock.
type orderLock struct {
	userID    int64
	locked    int64 // locked_remaining, this order's share of balances.locked
	remaining int64 // quantity not yet filled
}

func lockOrder(ctx context.Context, tx pgx.Tx, orderID int64) (orderLock, error) {
	var o orderLock
	err := tx.QueryRow(ctx, `
		SELECT user_id, locked_remaining, quantity - filled_quantity
		FROM orders
		WHERE id = $1
		FOR UPDATE
	`, orderID).Scan(&o.userID, &o.locked, &o.remaining)
	return o, err
}

// fillOrder advances one order's filled quantity and releases part of its lock.
//
// Status is derived from the new filled quantity rather than passed in, so it
// cannot disagree with the number it summarises. The orders_filled_within_quantity
// CHECK from migration 000003 is the backstop if a fill ever overshoots.
func fillOrder(ctx context.Context, tx pgx.Tx, orderID, quantity, release int64) error {
	_, err := tx.Exec(ctx, `
		UPDATE orders
		SET filled_quantity  = filled_quantity + $2,
		    locked_remaining = locked_remaining - $3,
		    status           = CASE WHEN filled_quantity + $2 >= quantity
		                            THEN $4 ELSE $5 END
		WHERE id = $1
	`, orderID, quantity, release, models.OrderFilled, models.OrderPartiallyFilled)
	return err
}

// moveBalance applies signed deltas to one balance row.
//
// Update first, and only insert when nothing was updated. This cannot be a
// single upsert: a CHECK constraint is evaluated against the PROPOSED insert
// row, before ON CONFLICT arbitration happens at all. A negative delta
// therefore trips balances_non_negative even when the row exists and the update
// would have landed on a perfectly valid figure. AdjustBalance carries the same
// note for the same reason — ON CONFLICT only arbitrates unique and exclusion
// violations, never CHECKs.
func moveBalance(ctx context.Context, tx pgx.Tx, userID int64, currency string, available, locked int64) error {
	if available == 0 && locked == 0 {
		return nil
	}

	tag, err := tx.Exec(ctx, `
		UPDATE balances
		SET available  = available + $3,
		    locked     = locked + $4,
		    updated_at = now()
		WHERE user_id = $1
		  AND currency = $2
	`, userID, currency, available, locked)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 1 {
		return nil
	}

	// Nothing to update. Only a credit can legitimately create a row — a buyer
	// receiving a currency they have never held. A debit against a balance that
	// does not exist means settlement is working from state the ledger does not
	// share, which is worth failing on rather than papering over.
	if available < 0 || locked < 0 {
		return fmt.Errorf("%w: user %d has no %s balance to debit", ErrNoBalance, userID, currency)
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO balances (user_id, currency, available, locked)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (user_id, currency) DO UPDATE
		SET available  = balances.available + $3,
		    locked     = balances.locked + $4,
		    updated_at = now()
	`, userID, currency, available, locked)
	return err
}

// FillsByUserID returns every execution the user was party to, newest first.
//
// The two halves are unioned rather than joined into one row because a user can
// be on both sides of the same trade — self-trading is not prevented — and that
// is two positions, not one. Collapsing them would silently drop half of a
// self-trade from the user's own record.
func (store *TradeStore) FillsByUserID(ctx context.Context, userID int64, limit int) (*models.Fills, error) {
	rows, err := store.pool.Query(ctx, `
		SELECT t.id, t.market, t.buy_order_id, 'buy',
		       t.quantity, t.price_each,
		       t.taker_order_id = t.buy_order_id,
		       t.executed_at
		FROM trades t
		JOIN orders o ON o.id = t.buy_order_id
		WHERE o.user_id = $1

		UNION ALL

		SELECT t.id, t.market, t.sell_order_id, 'sell',
		       t.quantity, t.price_each,
		       t.taker_order_id = t.sell_order_id,
		       t.executed_at
		FROM trades t
		JOIN orders o ON o.id = t.sell_order_id
		WHERE o.user_id = $1

		ORDER BY executed_at DESC, id DESC
		LIMIT $2
	`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	// Non-nil so an account with no fills encodes as [] rather than null.
	fills := models.Fills{Trades: make([]models.Fill, 0)}

	for rows.Next() {
		var fill models.Fill
		if err := rows.Scan(
			&fill.ID, &fill.Market, &fill.OrderID, &fill.Side,
			&fill.Quantity, &fill.Price, &fill.Taker, &fill.ExecutedAt,
		); err != nil {
			return nil, err
		}
		fills.Trades = append(fills.Trades, fill)
	}

	return &fills, rows.Err()
}

// RecentByMarket returns the public tape for one market, newest first.
//
// No join to orders: nothing here is attributable to a person, and keeping the
// query off that table is what makes it safe to serve without auth.
func (store *TradeStore) RecentByMarket(ctx context.Context, market string, limit int) (*models.MarketTrades, error) {
	rows, err := store.pool.Query(ctx, `
		SELECT id, market, quantity, price_each,
		       CASE WHEN taker_order_id = buy_order_id THEN 'buy' ELSE 'sell' END,
		       executed_at
		FROM trades
		WHERE market = $1
		ORDER BY executed_at DESC, id DESC
		LIMIT $2
	`, market, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	trades := models.MarketTrades{Trades: make([]models.MarketTrade, 0)}

	for rows.Next() {
		var trade models.MarketTrade
		if err := rows.Scan(
			&trade.ID, &trade.Market, &trade.Quantity, &trade.Price,
			&trade.TakerSide, &trade.ExecutedAt,
		); err != nil {
			return nil, err
		}
		trades.Trades = append(trades.Trades, trade)
	}

	return &trades, rows.Err()
}

// TickerByMarket summarises one market over the trailing windowHours.
//
// OpenPrice falls back to the first trade inside the window when the market has
// no history before it. Otherwise a market younger than the window would report
// its whole life as a change from zero, which reads as an infinite gain.
//
// Every aggregate is nullable — a market that has never traded has no last, no
// high and no low — so they are scanned into pointers and flattened here rather
// than COALESCEd to zero in SQL, which would make "no trades" indistinguishable
// from "traded at zero".
func (store *TradeStore) TickerByMarket(ctx context.Context, market string, windowHours int) (*models.Ticker, error) {
	ticker := models.Ticker{Market: market, WindowHours: windowHours}

	var last, before, first, high, low *int64
	var executedAt *time.Time

	err := store.pool.QueryRow(ctx, `
		WITH windowed AS (
			SELECT price_each, quantity, executed_at, id
			FROM trades
			WHERE market = $1
			  AND executed_at > now() - make_interval(hours => $2)
		)
		SELECT
			(SELECT price_each FROM trades
			  WHERE market = $1
			  ORDER BY executed_at DESC, id DESC LIMIT 1),
			(SELECT price_each FROM trades
			  WHERE market = $1
			    AND executed_at <= now() - make_interval(hours => $2)
			  ORDER BY executed_at DESC, id DESC LIMIT 1),
			(SELECT price_each FROM windowed ORDER BY executed_at ASC, id ASC LIMIT 1),
			(SELECT max(price_each) FROM windowed),
			(SELECT min(price_each) FROM windowed),
			(SELECT coalesce(sum(quantity), 0) FROM windowed),
			(SELECT count(*) FROM windowed),
			(SELECT max(executed_at) FROM trades WHERE market = $1)
	`, market, windowHours).Scan(
		&last, &before, &first, &high, &low,
		&ticker.BaseVolume, &ticker.TradeCount, &executedAt,
	)
	if err != nil {
		return nil, err
	}

	if last == nil {
		return &ticker, nil
	}

	ticker.HasTraded = true
	ticker.LastPrice = *last
	ticker.LastTradeAt = executedAt

	switch {
	case before != nil:
		ticker.OpenPrice = *before
	case first != nil:
		ticker.OpenPrice = *first
	default:
		// Traded, but not inside the window and not before it — impossible, since
		// every trade is one or the other. Treat the last price as flat.
		ticker.OpenPrice = *last
	}
	ticker.Change = ticker.LastPrice - ticker.OpenPrice

	if high != nil {
		ticker.High = *high
	}
	if low != nil {
		ticker.Low = *low
	}

	return &ticker, nil
}
