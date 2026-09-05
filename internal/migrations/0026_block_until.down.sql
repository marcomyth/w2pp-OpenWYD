-- Undo the ban expiry. Every timed ban becomes permanent, which is the
-- conservative direction: nobody is let in who was meant to be out.
ALTER TABLE account DROP COLUMN IF EXISTS blocked_until;
