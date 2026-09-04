-- Undo the admin panel schema.
--
-- The trigger goes first: it refuses UPDATE and DELETE on rows, and dropping the
-- table it guards has to be possible even so. DROP TABLE is DDL and is not
-- blocked by a row-level trigger, but removing the trigger before its function
-- keeps the order readable and the function unreferenced when it is dropped.
DROP TRIGGER IF EXISTS admin_audit_log_no_change ON admin_audit_log;
DROP FUNCTION IF EXISTS admin_audit_log_immutable();

DROP INDEX IF EXISTS admin_audit_log_target_idx;
DROP INDEX IF EXISTS admin_audit_log_created_at_idx;
DROP TABLE IF EXISTS admin_audit_log;

DROP INDEX IF EXISTS account_vip_until_idx;
ALTER TABLE account DROP COLUMN IF EXISTS vip_until;
