-- 0026_block_until — how long a ban lasts.
--
-- 0024 recorded WHY an account was blocked and deliberately left out the
-- expiry, because a timed ban would have been a promise the server could not
-- keep: nothing rechecked is_blocked after login, so a 24h ban on somebody
-- already playing could elapse without ever taking effect. The tmServer control
-- API changed that — a ban now ends the session — so the clock is honest.
--
-- Enforcement is by READ, not by a sweep. Every place that asks "is this account
-- blocked" applies the same expression (store.BlockedNowSQL), so a ban simply
-- stops counting the moment it expires. There is no job to run, nothing to fall
-- behind, and no window where a lifted ban still blocks a login.
--
-- NULL means the ban does not expire. It is not "expired in 1970": the three
-- readers treat NULL as permanent explicitly, because a sentinel past date would
-- make every permanent ban read as lifted.
ALTER TABLE account ADD COLUMN blocked_until TIMESTAMPTZ;

-- Nothing scans by this column — the expiry is evaluated per account, on rows
-- already found by name or id — so it gets no index.
