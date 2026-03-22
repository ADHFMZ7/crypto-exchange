-- add_available_and_locked_balances.down.sql

-- Recreate old column
ALTER TABLE balances
ADD COLUMN balance NUMERIC NOT NULL DEFAULT 0;

-- Restore data
UPDATE balances
SET balance = available + locked;

-- Remove constraint
ALTER TABLE balances
DROP CONSTRAINT IF EXISTS balances_non_negative;

-- Drop new columns
ALTER TABLE balances
DROP COLUMN available,
DROP COLUMN locked;
