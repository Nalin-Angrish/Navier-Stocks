ALTER TABLE trade_log DROP CONSTRAINT IF EXISTS fk_trade_log_position;
ALTER TABLE trade_log DROP COLUMN IF EXISTS position_id;
