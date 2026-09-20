-- 004 retired: UNIQUE on positions.execution_ref is already enforced by
-- 001_initial_schema (execution_ref VARCHAR(64) UNIQUE).
-- Retained as a no-op to preserve migration sequencing.
SELECT 1;
