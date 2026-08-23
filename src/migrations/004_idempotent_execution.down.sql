-- Rollback of 004: drop idempotency constraint and index.

DROP INDEX IF EXISTS idx_trade_log_execution_ref;
ALTER TABLE positions DROP CONSTRAINT IF EXISTS uq_positions_execution_ref;
