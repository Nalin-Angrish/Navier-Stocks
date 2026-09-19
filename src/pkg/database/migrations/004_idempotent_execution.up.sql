-- 004: Idempotent execution handling.
--
-- Adds a unique constraint on execution_ref to prevent duplicate position
-- inserts when JetStream redelivers messages after transient failures.
-- Also adds an index for the trader's lookup-before-insert pattern.

ALTER TABLE positions
    ADD CONSTRAINT uq_positions_execution_ref UNIQUE (execution_ref);

CREATE INDEX IF NOT EXISTS idx_trade_log_execution_ref ON trade_log(execution_ref);
