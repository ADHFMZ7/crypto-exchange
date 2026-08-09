package stores

import (
	"context"

	"github.com/ADHFMZ7/crypto-exchange/internal/models"
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
