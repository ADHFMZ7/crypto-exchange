-- settlement.up.sql
--
-- Everything settlement needs that the schema could not already express.

-- balances.locked is aggregated per (user, currency), so once a user holds two
-- open orders it cannot answer "how much of this lock belongs to THIS order".
-- Settlement has to know, because it releases the lock one fill at a time.
--
-- It also cannot be derived. The lock was rounded once, for the whole order, so
-- recomputing a per-fill share from quantity and price does not add back to it:
-- rounding each fill up overshoots and drives locked negative, rounding down
-- strands dust that nothing can ever release. Storing the remainder and
-- decrementing it makes the total exact by construction.
ALTER TABLE orders
ADD COLUMN locked_remaining BIGINT NOT NULL DEFAULT 0;

ALTER TABLE orders
ADD CONSTRAINT orders_locked_remaining_non_negative
CHECK (locked_remaining >= 0);

-- NUMERIC permits fractions, which is exactly what migration 000003 removed
-- from orders. trades was never given the same treatment, and it stores the
-- same two quantities in the same units.
ALTER TABLE trades
  ALTER COLUMN quantity   TYPE BIGINT USING quantity::BIGINT,
  ALTER COLUMN price_each TYPE BIGINT USING price_each::BIGINT;

-- Which order crossed the spread. Not reconstructable after the fact, and
-- settlement needs it: only the taker can execute better than its own limit, so
-- only the taker's lock can have money owed back to it.
--
-- NOT NULL with no default is safe here because nothing has ever written to
-- trades — the matching engine discarded its results until now.
ALTER TABLE trades
ADD COLUMN taker_order_id BIGINT NOT NULL REFERENCES orders(id);

ALTER TABLE trades
ADD CONSTRAINT trades_amounts_positive
CHECK (quantity > 0 AND price_each > 0);

-- Denormalised from orders.market, which both sides of a trade already agree
-- on. Without it every tape, ticker and candle query joins orders twice just to
-- filter by symbol — and those are the queries that will run most often.
ALTER TABLE trades
ADD COLUMN market TEXT NOT NULL;

CREATE INDEX idx_trades_market_executed_at ON trades(market, executed_at DESC);
