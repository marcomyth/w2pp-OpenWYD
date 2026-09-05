-- Undo the block reason columns. is_blocked stays: it predates this migration
-- and dropping it would unban everyone.
ALTER TABLE account DROP COLUMN IF EXISTS blocked_by;
ALTER TABLE account DROP COLUMN IF EXISTS blocked_at;
ALTER TABLE account DROP COLUMN IF EXISTS block_reason;
