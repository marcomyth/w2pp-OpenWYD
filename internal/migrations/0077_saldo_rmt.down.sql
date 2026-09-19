ALTER TABLE account DROP CONSTRAINT IF EXISTS account_rmt_balance_nao_negativo;
ALTER TABLE account DROP COLUMN IF EXISTS rmt_balance;
