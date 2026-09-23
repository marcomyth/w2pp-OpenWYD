DROP INDEX IF EXISTS rmt_cobranca_reembolso_aberto;
ALTER TABLE rmt_cobranca DROP COLUMN IF EXISTS reembolso_erro;
ALTER TABLE rmt_cobranca DROP COLUMN IF EXISTS reembolso_pedido_em;
ALTER TABLE rmt_cobranca DROP COLUMN IF EXISTS reembolso_status;
