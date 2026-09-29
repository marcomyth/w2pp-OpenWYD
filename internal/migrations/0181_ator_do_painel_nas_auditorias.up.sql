-- O USUÁRIO DO PAINEL NÃO CONSEGUE SALVAR NADA, E A CULPA É DESTAS QUATRO TABELAS.
--
-- Hoje o painel recusa toda edição de quem entra como usuário do painel. A recusa
-- está no interceptador do webServer, e o comentário dele diz por quê, em voz alta:
-- as escritas gravam o autor com a conta de JOGO, e um usuário do painel não tem
-- conta de jogo. Sem isto, a escrita passaria e registraria "conta 0" como autor.
-- Edição que funciona e mente sobre quem a fez é pior que edição recusada — e foi a
-- escolha certa enquanto a auditoria não soubesse o que é um usuário do painel.
--
-- É isso que esta migração conserta: as quatro tabelas de auditoria que as 23
-- escritas do painel usam passam a aceitar os dois tipos de ator, como a
-- admin_audit_log já aceita desde a 0130. A partir daqui a autorização pode ser
-- "usuário do painel ativo com cargo que pode a ação", sem inventar autor nenhum.
--
-- QUATRO TABELAS, E NÃO OITO OU NOVE. Foi medido escrita por escrita, e o mapa é:
--
--   npc_audit           18 escritas: as 5 de NPC, as 3 de monstro, as 2 de item base
--                       e as 6 de montaria. Todas passam pela MESMA função no
--                       internal/store (auditAndBump), que reusa npc_audit em vez de
--                       ter uma tabela por assunto.
--   donate_shop_audit    4: UpsertShopItem, SetShopItemEnabled, DeleteShopItem e
--                       CreditDonateBalance (esta com o item nulo).
--   daily_reward_audit   3: Upsert, SetEnabled e Delete de RewardItem.
--   world_event_audit    1: SetWorldEventConfig.
--
-- FICARAM DE FORA, e cada uma por um motivo conferido:
--   shop_points_audit    nenhuma das 23 escreve nela. O CreditDonateBalance atualiza
--                        account.donate_balance e audita em donate_shop_audit; a de
--                        pontos de loja é outro assunto.
--   TransformAttributeMap não grava linha em tabela nenhuma: ele lê o AttributeMap.dat
--                        e devolve um .dat novo para a pessoa trocar à mão. Não há
--                        autor para gravar.
--   admin_audit_log      já ganhou a coluna na 0130.
--
-- NAS DUAS DA LOJA E DA RECOMPENSA, A COLUNA account_id NÃO É SÓ DO MODERADOR: ela
-- também guarda o comprador de uma compra e quem resgatou a recompensa do dia. Isso
-- não muda nada aqui, e é a razão de a trava ser "exatamente um dos dois" e não "o
-- ator do painel é obrigatório": a linha do jogador continua tendo conta de jogo, e
-- só as linhas de staff sem personagem passam a ter o outro lado preenchido.

-- TODAS AS TRAVAS ENTRAM COMO "NOT VALID", E ISSO É O QUE IMPEDE ESTA MIGRAÇÃO DE
-- DERRUBAR A PRODUÇÃO NO BOOT.
--
-- Um ADD CONSTRAINT normal VARRE a tabela inteira e FALHA se achar uma linha que não
-- satisfaz. E existe linha assim hoje: desde 25/09 as telas de evento e de Kefra
-- gravam direto no store, e quando quem salva é um usuário do painel elas gravam
-- account_id NULO com fonte='painel' — exatamente o que a trava nova recusa. Uma
-- única dessas linhas faria o ADD CONSTRAINT falhar; e como o store.Migrate roda no
-- boot do webServer, do dbServer e do adminServer, a migração que falha não deixa
-- NENHUM dos três subir. O painel destravaria matando o servidor.
--
-- NOT VALID diz: "vale de agora em diante, não vou olhar o passado". As linhas novas
-- são conferidas normalmente, que é o que esta entrega precisa; as antigas ficam como
-- estão, e podem ser conferidas depois, com calma, por um VALIDATE CONSTRAINT que
-- roda sem travar a tabela.
--
-- E TEM UM SEGUNDO GANHO, que sozinho já justificaria: a varredura de um ADD
-- CONSTRAINT normal segura um ACCESS EXCLUSIVE na tabela enquanto roda. Três destas
-- quatro (donate_shop_audit, daily_reward_audit, world_event_audit) são escritas pelo
-- JOGO, com jogador dentro. Travá-las no meio de um deploy é parar compra, resgate e
-- evento por quanto tempo a varredura levar.
--
-- A TRAVA DO ZERO (account_id > 0) existe porque a de "um ator" não pega o caso que
-- originou tudo isto: zero NÃO é nulo, então uma linha com account_id = 0 passa por
-- ela como se tivesse autor. Hoje quem recusa o zero é só o Conferir() no código, e
-- código é o que muda; a trava fica no banco, que é onde a garantia dura.

-- npc_audit -----------------------------------------------------------------
ALTER TABLE npc_audit ALTER COLUMN account_id DROP NOT NULL;
ALTER TABLE npc_audit
    ADD COLUMN IF NOT EXISTS actor_painel_usuario_id BIGINT REFERENCES painel_usuario(id);

-- EXATAMENTE UM DOS DOIS, nunca os dois e nunca nenhum.
--
-- Sem esta trava, um erro de código escreveria uma linha de auditoria SEM ATOR — uma
-- ação que aconteceu e que ninguém consegue atribuir a ninguém. Numa tabela cuja
-- única razão de existir é dizer QUEM fez, isso é pior do que a linha não existir. É
-- a mesma trava que a 0130 pôs na admin_audit_log, com o mesmo nome de sufixo para
-- quem procurar uma achar as outras.
ALTER TABLE npc_audit
    ADD CONSTRAINT npc_audit_um_ator CHECK (
        (account_id IS NOT NULL AND actor_painel_usuario_id IS NULL)
        OR (account_id IS NULL AND actor_painel_usuario_id IS NOT NULL)
    ) NOT VALID;
ALTER TABLE npc_audit
    ADD CONSTRAINT npc_audit_conta_nao_zero CHECK (account_id IS NULL OR account_id > 0) NOT VALID;
CREATE INDEX IF NOT EXISTS npc_audit_ator_painel_idx
    ON npc_audit (actor_painel_usuario_id);

-- daily_reward_audit --------------------------------------------------------
ALTER TABLE daily_reward_audit ALTER COLUMN account_id DROP NOT NULL;
ALTER TABLE daily_reward_audit
    ADD COLUMN IF NOT EXISTS actor_painel_usuario_id BIGINT REFERENCES painel_usuario(id);
ALTER TABLE daily_reward_audit
    ADD CONSTRAINT daily_reward_audit_um_ator CHECK (
        (account_id IS NOT NULL AND actor_painel_usuario_id IS NULL)
        OR (account_id IS NULL AND actor_painel_usuario_id IS NOT NULL)
    ) NOT VALID;
ALTER TABLE daily_reward_audit
    ADD CONSTRAINT daily_reward_audit_conta_nao_zero CHECK (account_id IS NULL OR account_id > 0) NOT VALID;
CREATE INDEX IF NOT EXISTS daily_reward_audit_ator_painel_idx
    ON daily_reward_audit (actor_painel_usuario_id);

-- donate_shop_audit ---------------------------------------------------------
ALTER TABLE donate_shop_audit ALTER COLUMN account_id DROP NOT NULL;
ALTER TABLE donate_shop_audit
    ADD COLUMN IF NOT EXISTS actor_painel_usuario_id BIGINT REFERENCES painel_usuario(id);
ALTER TABLE donate_shop_audit
    ADD CONSTRAINT donate_shop_audit_um_ator CHECK (
        (account_id IS NOT NULL AND actor_painel_usuario_id IS NULL)
        OR (account_id IS NULL AND actor_painel_usuario_id IS NOT NULL)
    ) NOT VALID;
ALTER TABLE donate_shop_audit
    ADD CONSTRAINT donate_shop_audit_conta_nao_zero CHECK (account_id IS NULL OR account_id > 0) NOT VALID;
CREATE INDEX IF NOT EXISTS donate_shop_audit_ator_painel_idx
    ON donate_shop_audit (actor_painel_usuario_id);

-- world_event_audit ---------------------------------------------------------
--
-- ESTA TABELA TEM UMA TRAVA DIFERENTE DAS OUTRAS TRÊS, e a diferença é obrigatória:
-- aqui existe uma linha LEGÍTIMA sem ator nenhum.
--
-- Desde a 0067 o account_id já aceita nulo, porque o JOGO escreve nesta tabela: quando
-- o Kefra cai, o servidor grava a mudança de estado com fonte='jogo' e sem moderador —
-- não houve pessoa. Se eu copiasse aqui o "exatamente um dos dois" das outras, TODA
-- gravação do jogo passaria a ser recusada, e a cidade do Kefra pararia de abrir. A
-- migração derrubaria uma parte do jogo em vez de destravar o painel.
--
-- ENTÃO A REGRA AQUI É "NUNCA OS DOIS", e o resto fica amarrado pela fonte: quando a
-- linha veio do painel, alguém a fez e um dos dois lados tem de estar preenchido;
-- quando veio do jogo, os dois são nulos. Isso continua impedindo o defeito que esta
-- migração existe para impedir — atribuir a ação a quem não a fez — sem inventar uma
-- pessoa para o que o servidor fez sozinho.
ALTER TABLE world_event_audit
    ADD COLUMN IF NOT EXISTS actor_painel_usuario_id BIGINT REFERENCES painel_usuario(id);
ALTER TABLE world_event_audit
    ADD CONSTRAINT world_event_audit_um_ator CHECK (
        NOT (account_id IS NOT NULL AND actor_painel_usuario_id IS NOT NULL)
        AND (
            fonte <> 'painel'
            OR account_id IS NOT NULL
            OR actor_painel_usuario_id IS NOT NULL
        )
    ) NOT VALID;
ALTER TABLE world_event_audit
    ADD CONSTRAINT world_event_audit_conta_nao_zero CHECK (account_id IS NULL OR account_id > 0) NOT VALID;
CREATE INDEX IF NOT EXISTS world_event_audit_ator_painel_idx
    ON world_event_audit (actor_painel_usuario_id);
