-- ledger_outbox.up.sql
--
-- A durable queue for the effects the matching engine produces.
--
-- The engine decides that two orders traded, or that one stopped resting, and
-- the settlement worker turns that into rows and balances. Until now the only
-- record of a decision in flight was a Go channel: if applying it failed, the
-- worker logged a line and moved on, and the book and the ledger disagreed
-- permanently. A transient database blip was enough to lose a fill for good.
--
-- Recording the decision before acting on it turns that failure into a delay.
-- The row stays pending until it applies, survives a restart, and — when it
-- genuinely cannot be applied — is left behind as evidence rather than a log
-- line nobody will find.
CREATE TABLE ledger_events (
    id         BIGSERIAL PRIMARY KEY,
    market     TEXT NOT NULL,
    kind       TEXT NOT NULL,
    -- JSONB rather than columns per kind: a fill and a cancellation carry
    -- different shapes, and this table is an append-only record of what the
    -- engine said, never something queried by field. The shapes are
    -- models.Trade and models.OrderCancel, which already define their own wire
    -- form. Amounts inside stay integer minor units like everywhere else.
    payload    JSONB NOT NULL,

    attempts   INT NOT NULL DEFAULT 0,
    last_error TEXT,

    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    applied_at TIMESTAMPTZ,
    failed_at  TIMESTAMPTZ,

    CONSTRAINT ledger_events_kind_valid
        CHECK (kind IN ('fill', 'cancel')),

    -- An event is pending, applied, or given up on. Never two of those.
    CONSTRAINT ledger_events_one_outcome
        CHECK (applied_at IS NULL OR failed_at IS NULL)
);

-- The only hot query is "what is still owed", which stays small however large
-- the table grows. A partial index keeps that scan proportional to the backlog
-- rather than to the history.
CREATE INDEX idx_ledger_events_pending
    ON ledger_events (id)
    WHERE applied_at IS NULL AND failed_at IS NULL;

-- Dead letters are read by a person, after an alert, in whatever order they
-- happened.
CREATE INDEX idx_ledger_events_failed
    ON ledger_events (failed_at)
    WHERE failed_at IS NOT NULL;
