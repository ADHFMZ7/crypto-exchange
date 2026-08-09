-- add_filled_quantity.up.sql
--
-- Adds the running fill total to orders, and tightens the columns around it.
--
-- filled_quantity is a denormalised summary of the trades ledger: once
-- executions exist it equals SUM(trades.quantity) for the order. It is stored
-- rather than derived so GET /orders is a plain SELECT instead of a join and
-- aggregate over an append-only table. The cost of that choice is that it must
-- be updated in the SAME transaction as the trade insert, or it drifts from the
-- ledger it summarises.

ALTER TABLE orders
ADD COLUMN filled_quantity BIGINT NOT NULL DEFAULT 0;

-- NUMERIC permits fractions, which reopens the door integer minor units exist
-- to close. BIGINT matches the int64 these are already scanned into, and makes
-- a fractional quantity unrepresentable rather than merely unexpected.
ALTER TABLE orders
  ALTER COLUMN quantity   TYPE BIGINT USING quantity::BIGINT,
  ALTER COLUMN price_each TYPE BIGINT USING price_each::BIGINT;

-- An order can never fill for more than it asked for. This is the invariant
-- settlement is most likely to break, so it belongs in the schema rather than
-- in a rule the settlement code has to remember.
ALTER TABLE orders
ADD CONSTRAINT orders_filled_within_quantity
CHECK (filled_quantity >= 0 AND filled_quantity <= quantity);

-- Existing rows were written as 'OPEN' while the schema comment said lowercase.
-- Normalise before constraining, or the constraint fails on its own table.
UPDATE orders SET status = lower(status);

-- partially_filled only becomes expressible now that filled_quantity exists:
-- without it there is no way to tell a resting order from a half-filled one.
ALTER TABLE orders
ADD CONSTRAINT orders_status_valid
CHECK (status IN ('open', 'partially_filled', 'filled', 'cancelled'));
