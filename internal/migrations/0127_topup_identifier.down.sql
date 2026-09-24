DROP INDEX IF EXISTS donate_topup_order_a_conferir;
DROP INDEX IF EXISTS donate_topup_order_identifier;
ALTER TABLE donate_topup_order DROP COLUMN IF EXISTS gateway_identifier;
