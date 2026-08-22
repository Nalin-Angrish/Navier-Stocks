-- Rollback of 003: drop exit-accounting columns and their index.

DROP INDEX IF EXISTS idx_positions_closed_at;

ALTER TABLE positions
    DROP COLUMN IF EXISTS exit_reason,
    DROP COLUMN IF EXISTS pnl,
    DROP COLUMN IF EXISTS exit_price;
