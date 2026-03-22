-- add_available_and_locked_balances.up.sql

ALTER TABLE balances
ADD COLUMN available NUMERIC NOT NULL DEFAULT 0,
ADD COLUMN locked NUMERIC NOT NULL DEFAULT 0;

-- Migrate existing data
UPDATE balances
SET available = balance;

-- Drop old column
ALTER TABLE balances
DROP COLUMN balance;

-- Enforce invariants
ALTER TABLE balances
ADD CONSTRAINT balances_non_negative
CHECK (available >= 0 AND locked >= 0);
