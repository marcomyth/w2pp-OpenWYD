-- Volta a 0067. As linhas de auditoria gravadas pelo jogo não têm conta e precisam
-- sair para o account_id voltar a ser obrigatório.
DELETE FROM world_event_audit WHERE account_id IS NULL;
ALTER TABLE world_event_audit
    DROP COLUMN IF EXISTS fonte,
    ALTER COLUMN account_id SET NOT NULL;
ALTER TABLE world_event_config DROP COLUMN IF EXISTS kefra_guild_id;
