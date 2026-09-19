-- 003: Exit accounting for positions.
--
-- Records the outcome of every close so realized P&L can be reported per
-- trade, per day, and per exit reason.  Columns stay NULL for open rows
-- and for legacy closed rows predating this migration (backfill-safe).

ALTER TABLE positions
    ADD COLUMN IF NOT EXISTS exit_price  DOUBLE PRECISION,
    ADD COLUMN IF NOT EXISTS pnl         DOUBLE PRECISION,
    ADD COLUMN IF NOT EXISTS exit_reason VARCHAR(16)
        CHECK (exit_reason IN ('stop_loss', 'take_profit', 'square_off'));

CREATE INDEX IF NOT EXISTS idx_positions_closed_at ON positions(closed_at)
    WHERE status = 'CLOSED';
