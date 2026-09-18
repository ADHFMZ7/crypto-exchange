-- ledger_outbox.down.sql

DROP INDEX IF EXISTS idx_ledger_events_failed;
DROP INDEX IF EXISTS idx_ledger_events_pending;
DROP TABLE IF EXISTS ledger_events;
