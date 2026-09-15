-- 0067_kefra_estado — o estado do Kefra ganha uma fonte só, gravável pelo jogo.
--
-- Até aqui a chave do Kefra era mexida só pelo painel, junto com o formulário
-- inteiro dos eventos. No ciclo do Kefra (os jogadores matam o chefe e a XP fica
-- inteira; na terça ele volta e a XP cai pela metade), quem grava o estado é o
-- jogo, e o painel vira correção manual. Esta migração prepara as duas coisas que
-- faltam, sem mudar o estado:
--
--   * kefra_guild_id: a guilda que matou o Kefra (0 = sem guilda, ou Kefra vivo),
--     que o legado guarda dentro do próprio KefraLive e mostra no login;
--   * world_event_audit.fonte, com account_id aceitando nulo: a gravação feita pelo
--     jogo não tem conta de moderador, e a auditoria precisa dizer de onde veio.

ALTER TABLE world_event_config
    ADD COLUMN kefra_guild_id INTEGER NOT NULL DEFAULT 0 CHECK (kefra_guild_id >= 0);

ALTER TABLE world_event_audit
    ALTER COLUMN account_id DROP NOT NULL,
    ADD COLUMN fonte TEXT NOT NULL DEFAULT 'painel';
