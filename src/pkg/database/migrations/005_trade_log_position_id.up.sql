-- Add position_id foreign key to trade_log (OBS-22).
-- Existing rows get NULL (no backfill); new inserts must supply it.

ALTER TABLE trade_log
    ADD COLUMN IF NOT EXISTS position_id BIGINT;

ALTER TABLE trade_log
    ADD CONSTRAINT fk_trade_log_position
    FOREIGN KEY (position_id) REFERENCES positions(id)
    ON DELETE SET NULL;
