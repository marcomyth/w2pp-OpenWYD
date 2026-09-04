-- Schema for the staff admin panel (adminserver): the VIP axis and the audit log.
--
-- Both changes are additive and unread by any game service, which is what keeps
-- the panel deletable: dropping the service leaves these two objects inert rather
-- than leaving the game depending on a feature that is gone.
--
-- They ship together, in one migration, for an operational reason. Every service
-- embeds this directory, so touching it redeploys tmServer, dbServer, binServer
-- and webServer — a restart of the live game. One migration is one restart; two
-- would be two, for the same result.

-- VIP is an entitlement with an expiry, NOT a rung on the role ladder.
--
-- Roles are ordered and compared with >= (world.AccessLevel): player, moderator,
-- admin. Slotting VIP in there would make every GM a VIP by construction, give a
-- paying player authority over a non-paying one, and — because entitlements
-- expire and authority does not — force an expiry to DEMOTE somebody, taking the
-- GM away from whoever was one.
--
-- NULL, or a timestamp in the past, means "not VIP". Nothing has to clean up
-- after an expiry: readers compare against now(), and restoring the benefit is
-- moving the date forward.
ALTER TABLE account
    ADD COLUMN IF NOT EXISTS vip_until TIMESTAMPTZ;

-- Partial: the index only has to answer "who is VIP", and on a server where most
-- accounts never buy anything, most rows are NULL and do not belong in it.
CREATE INDEX IF NOT EXISTS account_vip_until_idx
    ON account (vip_until) WHERE vip_until IS NOT NULL;

-- Every administrative write, recorded before it is trusted.
CREATE TABLE IF NOT EXISTS admin_audit_log (
    id                BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    actor_account_id  BIGINT NOT NULL REFERENCES account(id),
    -- The actor's role AT THE TIME of the action. Without it, demoting someone
    -- retroactively rewrites how every past action of theirs reads.
    actor_role        TEXT   NOT NULL,
    action            TEXT   NOT NULL,  -- SET_ROLE, SET_VIP, SET_BLOCKED, …
    -- Nullable: an action need not have a target (a future server-wide setting).
    target_account_id BIGINT REFERENCES account(id),
    -- Before and after, as sent. JSONB so a new action type does not need a
    -- migration to record what it changed.
    old_value         JSONB,
    new_value         JSONB,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Reading the log is always "what happened, newest first", optionally narrowed to
-- one account. Two indexes rather than one composite: the target filter and the
-- unfiltered timeline are separate questions.
CREATE INDEX IF NOT EXISTS admin_audit_log_created_at_idx
    ON admin_audit_log (created_at DESC);
CREATE INDEX IF NOT EXISTS admin_audit_log_target_idx
    ON admin_audit_log (target_account_id, created_at DESC)
    WHERE target_account_id IS NOT NULL;

-- Append-only, enforced by the database.
--
-- "No endpoint issues UPDATE or DELETE" is a convention, and the first person
-- with a psql prompt is outside it — which is exactly the person an audit log
-- exists to record. A trigger refuses regardless of who is asking.
CREATE OR REPLACE FUNCTION admin_audit_log_immutable() RETURNS TRIGGER AS $$
BEGIN
    RAISE EXCEPTION 'admin_audit_log is append-only (attempted %)', TG_OP;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS admin_audit_log_no_change ON admin_audit_log;
CREATE TRIGGER admin_audit_log_no_change
    BEFORE UPDATE OR DELETE ON admin_audit_log
    FOR EACH ROW EXECUTE FUNCTION admin_audit_log_immutable();
