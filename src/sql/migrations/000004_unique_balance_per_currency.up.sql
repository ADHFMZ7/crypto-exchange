-- unique_balance_per_currency.up.sql
--
-- A user holds exactly one balance per currency. Nothing enforced that, which
-- left two doors open: a duplicate row would split a holding in two (each query
-- reading whichever it found first), and without a unique key there is no
-- conflict target for an upsert — so crediting a currency the user has never
-- held required a read-then-write race instead of one atomic statement.

ALTER TABLE balances
ADD CONSTRAINT balances_user_currency_unique UNIQUE (user_id, currency);
