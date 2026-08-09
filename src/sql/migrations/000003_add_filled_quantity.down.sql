-- add_filled_quantity.down.sql

ALTER TABLE orders
DROP CONSTRAINT IF EXISTS orders_status_valid;

ALTER TABLE orders
DROP CONSTRAINT IF EXISTS orders_filled_within_quantity;

-- Restore the casing 000002-era code wrote.
UPDATE orders SET status = upper(status);

ALTER TABLE orders
  ALTER COLUMN quantity   TYPE NUMERIC USING quantity::NUMERIC,
  ALTER COLUMN price_each TYPE NUMERIC USING price_each::NUMERIC;

ALTER TABLE orders
DROP COLUMN filled_quantity;
