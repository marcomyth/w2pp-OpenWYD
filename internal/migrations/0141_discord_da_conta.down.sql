DROP INDEX IF EXISTS account_discord_id_unico;
ALTER TABLE account DROP CONSTRAINT IF EXISTS account_discord_id_nao_vazio;
ALTER TABLE account DROP COLUMN discord_id;
