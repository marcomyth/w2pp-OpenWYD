-- A VOLTA RECUSA EM VEZ DE APAGAR AUDITORIA.
--
-- ESTA É A DIFERENÇA DELIBERADA EM RELAÇÃO À 0130, que na volta apaga as linhas sem
-- conta de jogo. Aqui não: se existir uma única ação feita por usuário do painel,
-- esta migração FALHA e diz qual tabela e quantas linhas.
--
-- O motivo é o que essas linhas são. Numa tabela de auditoria, apagar é apagar a
-- prova de que alguém mexeu em preço de item, em recompensa do dia, em loja de
-- doação ou em evento de mundo. Uma volta que faz isso em silêncio transforma
-- "desfazer uma migração" em "sumir com o histórico", e quem rodou não fica sabendo.
--
-- Recusar é chato e é de propósito: quem REALMENTE quiser voltar decide o que fazer
-- com essas linhas — exportar, mover para outro lugar, ou apagar à mão sabendo o que
-- está apagando. A decisão fica com uma pessoa, e não com um arquivo .sql.

DO $$
DECLARE
    t     TEXT;
    n     BIGINT;
    sobra TEXT := '';
BEGIN
    FOREACH t IN ARRAY ARRAY['npc_audit', 'daily_reward_audit',
                             'donate_shop_audit', 'world_event_audit']
    LOOP
        EXECUTE format('SELECT count(*) FROM %I WHERE actor_painel_usuario_id IS NOT NULL', t)
            INTO n;
        IF n > 0 THEN
            sobra := sobra || format('%s: %s linha(s); ', t, n);
        END IF;
    END LOOP;

    IF sobra <> '' THEN
        RAISE EXCEPTION
            'a volta da 0181 apagaria auditoria de usuario do painel -- %',
            sobra
            USING HINT = 'decida o que fazer com essas linhas (exportar, mover ou '
                      || 'apagar a mao) e rode a volta depois; nenhum .sql vai '
                      || 'apagar prova de auditoria em silencio';
    END IF;
END $$;

ALTER TABLE world_event_audit DROP CONSTRAINT IF EXISTS world_event_audit_um_ator;
ALTER TABLE world_event_audit DROP CONSTRAINT IF EXISTS world_event_audit_conta_nao_zero;
DROP INDEX IF EXISTS world_event_audit_ator_painel_idx;
ALTER TABLE world_event_audit DROP COLUMN IF EXISTS actor_painel_usuario_id;
-- O NOT NULL NÃO VOLTA AQUI, e não é esquecimento: quem tirou o NOT NULL desta coluna
-- foi a 0067, para o jogo poder gravar sem moderador. Pôr de volta desfaria a 0067 e
-- quebraria a gravação do Kefra — a volta de uma migração não pode desfazer outra.

ALTER TABLE donate_shop_audit DROP CONSTRAINT IF EXISTS donate_shop_audit_um_ator;
ALTER TABLE donate_shop_audit DROP CONSTRAINT IF EXISTS donate_shop_audit_conta_nao_zero;
DROP INDEX IF EXISTS donate_shop_audit_ator_painel_idx;
ALTER TABLE donate_shop_audit DROP COLUMN IF EXISTS actor_painel_usuario_id;
ALTER TABLE donate_shop_audit ALTER COLUMN account_id SET NOT NULL;

ALTER TABLE daily_reward_audit DROP CONSTRAINT IF EXISTS daily_reward_audit_um_ator;
ALTER TABLE daily_reward_audit DROP CONSTRAINT IF EXISTS daily_reward_audit_conta_nao_zero;
DROP INDEX IF EXISTS daily_reward_audit_ator_painel_idx;
ALTER TABLE daily_reward_audit DROP COLUMN IF EXISTS actor_painel_usuario_id;
ALTER TABLE daily_reward_audit ALTER COLUMN account_id SET NOT NULL;

ALTER TABLE npc_audit DROP CONSTRAINT IF EXISTS npc_audit_um_ator;
ALTER TABLE npc_audit DROP CONSTRAINT IF EXISTS npc_audit_conta_nao_zero;
DROP INDEX IF EXISTS npc_audit_ator_painel_idx;
ALTER TABLE npc_audit DROP COLUMN IF EXISTS actor_painel_usuario_id;
ALTER TABLE npc_audit ALTER COLUMN account_id SET NOT NULL;
