package stores

import (
	"context"
	"errors"

	"github.com/ADHFMZ7/crypto-exchange/internal/models"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type OrderStore struct {
	pool *pgxpool.Pool
}

// GetByUserID returns every order belonging to a user, newest first.
//
// Ordering is part of the contract: the UI lists recent submissions, so a
// non-deterministic order would reshuffle the page on every refresh. The
// tiebreak on id keeps it stable for orders created in the same instant.
//
// filled_quantity reads 0 for every row until the matching engine reports
// fills. That is accurate rather than a placeholder — nothing can fill yet.
func (store *OrderStore) GetByUserID(ctx context.Context, userID int64) (*models.Orders, error) {
	rows, err := store.pool.Query(ctx, `
		SELECT id, market, side, quantity, filled_quantity, price_each, status, created_at
		FROM orders
		WHERE user_id = $1
		ORDER BY created_at DESC, id DESC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	// Non-nil so a user with no orders encodes as "orders": [] rather than
	// "orders": null, which the client cannot read as an array.
	orders := models.Orders{Orders: make([]models.Order, 0)}

	for rows.Next() {
		var order models.Order
		if err := rows.Scan(
			&order.ID,
			&order.Market,
			&order.Side,
			&order.Quantity,
			&order.FilledQuantity,
			&order.PriceEach,
			&order.Status,
			&order.CreatedAt,
		); err != nil {
			return nil, err
		}
		orders.Orders = append(orders.Orders, order)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return &orders, nil
}

// ErrOrderNotFound covers both "no such order" and "not this user's order".
//
// The two are deliberately indistinguishable to a caller: telling them apart
// would let anyone probe which order ids exist by watching the status code
// change.
var ErrOrderNotFound = errors.New("order not found")

// GetByIDForUser returns one order, but only to the user who placed it.
//
// Ownership is part of the WHERE clause rather than checked after the read, so
// there is no window in which a caller holds another user's order at all.
func (store *OrderStore) GetByIDForUser(ctx context.Context, orderID, userID int64) (*models.Order, error) {
	var order models.Order

	err := store.pool.QueryRow(ctx, `
		SELECT id, market, side, quantity, filled_quantity, price_each, status, created_at
		FROM orders
		WHERE id = $1 AND user_id = $2
	`, orderID, userID).Scan(
		&order.ID,
		&order.Market,
		&order.Side,
		&order.Quantity,
		&order.FilledQuantity,
		&order.PriceEach,
		&order.Status,
		&order.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrOrderNotFound
	}
	if err != nil {
		return nil, err
	}

	return &order, nil
}

// RestingOrders returns every order the book would still be matching, oldest
// first.
//
// Oldest first because the caller is reconstructing or unwinding book state,
// and both want the order things entered in. It is the commit order rather than
// the order the matching worker saw them in — close, but not the same thing,
// which is why nothing here claims to restore price-time priority.
func (store *OrderStore) RestingOrders(ctx context.Context) ([]models.Order, error) {
	rows, err := store.pool.Query(ctx, `
		SELECT id, market, side, quantity, filled_quantity, price_each, status, created_at
		FROM orders
		WHERE status IN ($1, $2)
		ORDER BY created_at ASC, id ASC
	`, models.OrderOpen, models.OrderPartiallyFilled)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	orders := make([]models.Order, 0)

	for rows.Next() {
		var order models.Order
		if err := rows.Scan(
			&order.ID,
			&order.Market,
			&order.Side,
			&order.Quantity,
			&order.FilledQuantity,
			&order.PriceEach,
			&order.Status,
			&order.CreatedAt,
		); err != nil {
			return nil, err
		}
		orders = append(orders, order)
	}

	return orders, rows.Err()
}
