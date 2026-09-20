-- Add position_id foreign key to trade_log (OBS-22).
-- Existing rows get NULL (no backfill); new inserts must supply it.

ALTER TABLE trade_log
    ADD COLUMN IF NOT EXISTS position_id BIGINT;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'fk_trade_log_position'
    ) THEN
        ALTER TABLE trade_log
            ADD CONSTRAINT fk_trade_log_position
            FOREIGN KEY (position_id) REFERENCES positions(id)
            ON DELETE SET NULL;
    END IF;
END $$;
