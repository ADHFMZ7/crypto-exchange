package stores

import (
	"context"
	"encoding/json"

	"github.com/ADHFMZ7/crypto-exchange/internal/models"
	"github.com/jackc/pgx/v5/pgxpool"
)

// OutboxStore is the durable queue of effects the matching engine has produced
// and the ledger has not yet absorbed.
//
// It exists because the engine's decisions used to live only in a channel. A
// fill that failed to settle was logged and dropped, which left the book
// believing a trade had happened that the ledger had no record of — silently,
// permanently, and from nothing worse than a moment's database trouble.
type OutboxStore struct {
	pool *pgxpool.Pool
}

// PendingEvent is one recorded effect, still owed to the ledger.
type PendingEvent struct {
	ID      int64
	Kind    string
	Payload []byte
}

// Append records an effect before anything tries to apply it.
//
// The id it returns is how every later step refers to the event, so a crash
// between recording and applying leaves something to find rather than nothing.
func (store *OutboxStore) Append(ctx context.Context, event models.LedgerEvent) (int64, error) {
	kind, payload, market, err := encodeEvent(event)
	if err != nil {
		return 0, err
	}

	var id int64
	err = store.pool.QueryRow(ctx, `
		INSERT INTO ledger_events (market, kind, payload)
		VALUES ($1, $2, $3)
		RETURNING id
	`, market, kind, payload).Scan(&id)

	return id, err
}

// Pending returns events still owed to the ledger, oldest first.
//
// Order is the contract, not a convenience: a cancellation that overtook its
// own fill would zero a lock the fill still needs. Replaying by id preserves
// the order the engine produced them in.
func (store *OutboxStore) Pending(ctx context.Context, limit int) ([]PendingEvent, error) {
	rows, err := store.pool.Query(ctx, `
		SELECT id, kind, payload
		FROM ledger_events
		WHERE applied_at IS NULL
		  AND failed_at IS NULL
		ORDER BY id ASC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	events := make([]PendingEvent, 0)
	for rows.Next() {
		var event PendingEvent
		if err := rows.Scan(&event.ID, &event.Kind, &event.Payload); err != nil {
			return nil, err
		}
		events = append(events, event)
	}

	return events, rows.Err()
}

// MarkApplied closes an event out. The ledger now agrees with the book about it.
func (store *OutboxStore) MarkApplied(ctx context.Context, id int64) error {
	_, err := store.pool.Exec(ctx,
		`UPDATE ledger_events SET applied_at = now() WHERE id = $1 AND applied_at IS NULL`, id)
	return err
}

// RecordAttempt notes a failure without giving up on the event.
func (store *OutboxStore) RecordAttempt(ctx context.Context, id int64, cause string) error {
	_, err := store.pool.Exec(ctx,
		`UPDATE ledger_events SET attempts = attempts + 1, last_error = $2 WHERE id = $1`,
		id, cause)
	return err
}

// MarkFailed gives up on an event and leaves it where a person will find it.
//
// This is the acknowledgement that the book and the ledger now disagree about
// something specific and repairable — which is the whole reason for writing
// these rows down rather than logging them.
func (store *OutboxStore) MarkFailed(ctx context.Context, id int64, cause string) error {
	_, err := store.pool.Exec(ctx, `
		UPDATE ledger_events
		SET failed_at = now(), attempts = attempts + 1, last_error = $2
		WHERE id = $1 AND applied_at IS NULL
	`, id, cause)
	return err
}

// FailedCount reports how many events were given up on — the number that should
// be zero, and that something should shout about when it is not.
func (store *OutboxStore) FailedCount(ctx context.Context) (int, error) {
	var n int
	err := store.pool.QueryRow(ctx,
		`SELECT count(*) FROM ledger_events WHERE failed_at IS NOT NULL`).Scan(&n)
	return n, err
}

// encodeEvent flattens the sum type into the columns the table holds.
func encodeEvent(event models.LedgerEvent) (kind string, payload []byte, market string, err error) {
	switch {
	case event.Fill != nil:
		payload, err = json.Marshal(event.Fill)
		return models.LedgerFill, payload, event.Fill.Market, err
	case event.Cancel != nil:
		payload, err = json.Marshal(event.Cancel)
		return models.LedgerCancel, payload, event.Cancel.Market, err
	}
	return "", nil, "", models.ErrEmptyLedgerEvent
}

// DecodeEvent rebuilds a recorded event. The kind column decides which half of
// the sum type the payload is.
func DecodeEvent(kind string, payload []byte) (models.LedgerEvent, error) {
	switch kind {
	case models.LedgerFill:
		var trade models.Trade
		if err := json.Unmarshal(payload, &trade); err != nil {
			return models.LedgerEvent{}, err
		}
		return models.LedgerEvent{Fill: &trade}, nil

	case models.LedgerCancel:
		var cancel models.OrderCancel
		if err := json.Unmarshal(payload, &cancel); err != nil {
			return models.LedgerEvent{}, err
		}
		return models.LedgerEvent{Cancel: &cancel}, nil
	}
	return models.LedgerEvent{}, models.ErrEmptyLedgerEvent
}
