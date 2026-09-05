-- Undo character presence.
DROP INDEX IF EXISTS character_online_since_idx;
ALTER TABLE character DROP COLUMN IF EXISTS online_since;
