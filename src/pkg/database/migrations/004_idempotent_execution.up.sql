-- 004: Idempotent execution handling.
--
-- Adds a unique constraint on execution_ref to prevent duplicate position
-- inserts when JetStream redelivers messages after transient failures.
-- Also adds an index for the trader's lookup-before-insert pattern.

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'uq_positions_execution_ref'
    ) THEN
        ALTER TABLE positions
            ADD CONSTRAINT uq_positions_execution_ref UNIQUE (execution_ref);
    END IF;
END $$;

CREATE INDEX IF NOT EXISTS idx_trade_log_execution_ref ON trade_log(execution_ref);
