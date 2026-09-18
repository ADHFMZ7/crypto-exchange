-- settlement.down.sql

DROP INDEX IF EXISTS idx_trades_market_executed_at;

ALTER TABLE trades DROP COLUMN IF EXISTS market;

ALTER TABLE trades DROP CONSTRAINT IF EXISTS trades_amounts_positive;

ALTER TABLE trades DROP COLUMN IF EXISTS taker_order_id;

ALTER TABLE trades
  ALTER COLUMN quantity   TYPE NUMERIC,
  ALTER COLUMN price_each TYPE NUMERIC;

ALTER TABLE orders DROP CONSTRAINT IF EXISTS orders_locked_remaining_non_negative;

ALTER TABLE orders DROP COLUMN IF EXISTS locked_remaining;
