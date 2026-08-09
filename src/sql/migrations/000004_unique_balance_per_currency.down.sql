-- unique_balance_per_currency.down.sql

ALTER TABLE balances
DROP CONSTRAINT IF EXISTS balances_user_currency_unique;
