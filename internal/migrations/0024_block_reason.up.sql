-- 0024_block_reason — why an account was blocked, who did it, and when.
--
-- Until now account.is_blocked was a bare boolean. A player who wrote in asking
-- why they could not log in got an answer only if whoever banned them happened
-- to remember, and the panel's audit log covers only panel bans: the in-game
-- /gm ban path (dbserver SetBlockedByName) writes no audit row at all, so those
-- bans had no recorded reason anywhere.
--
-- Deliberately NOT here: an expiry. A timed ban would be a half-truth today —
-- nothing rechecks is_blocked after account login, so a 24h ban on somebody
-- already online can elapse without ever taking effect. The expiry lands
-- together with the tmServer control link that can end a live session, and the
-- column arrives with it, so nobody can set a date the server will not honour.
--
-- Also not here: showing the reason to the player. dbServer checks is_blocked
-- BEFORE verifying the password, so a reason on the login screen would be
-- readable by anyone who types an account name with any password at all.

ALTER TABLE account ADD COLUMN block_reason TEXT NOT NULL DEFAULT '';

-- Nullable, because the great majority of rows were never blocked and a
-- sentinel date would have to be invented for them.
ALTER TABLE account ADD COLUMN blocked_at TIMESTAMPTZ;

-- ON DELETE SET NULL, never CASCADE: deleting the moderator who issued a ban
-- must not delete the banned account.
ALTER TABLE account ADD COLUMN blocked_by BIGINT REFERENCES account(id) ON DELETE SET NULL;
