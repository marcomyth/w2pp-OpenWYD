-- 0028_player_report — o que o jogador escreveu e o que estava acontecendo.
--
-- Não existia nada. Um jogador com problema mandava print e contava a história,
-- e quem atendia tinha que acreditar ou pedir para reproduzir. O comando
-- /reportar no jogo grava aqui o momento: quem, onde, com o que, e quem estava
-- por perto.
--
-- Sem chave estrangeira para character ou account, pelo mesmo motivo do
-- 0025_trade_log: DeleteCharacter é do jogador e apaga a linha de verdade, então
-- CASCADE deixaria o denunciado apagar a denúncia e RESTRICT deixaria ele travar
-- a própria exclusão dentro de um chamado. Nomes e ids entram como valor.
--
-- PRAZO. A linha nasce com expira_em e some depois disso: é texto que uma pessoa
-- escreveu e uma lista de quem estava por perto — gente que não pediu para estar
-- em lugar nenhum. O que o painel mostra é sempre filtrado por esse prazo, e as
-- linhas vencidas são apagadas quando a lista é lida. Isso é honesto sobre o que
-- garante: não aparece depois do prazo, e sai do banco na próxima vez que
-- alguém abrir a página. Não existe rotina periódica no projeto para prometer
-- mais do que isso.

CREATE TABLE player_report (
    id          BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    criado_em   TIMESTAMPTZ NOT NULL DEFAULT now(),
    expira_em   TIMESTAMPTZ NOT NULL,

    -- Quem reportou. Copiados, não referenciados.
    conta_id  BIGINT,
    conta     TEXT NOT NULL,
    char_nome TEXT NOT NULL,
    char_nivel INTEGER NOT NULL DEFAULT 0,

    -- O que a pessoa escreveu. É o relato; sem ele não há denúncia nenhuma.
    texto TEXT NOT NULL,

    -- Onde estava. Sem isso o relato vira "em algum lugar do mapa".
    pos_x INTEGER NOT NULL DEFAULT 0,
    pos_y INTEGER NOT NULL DEFAULT 0,

    -- Quem estava à vista naquele instante, só os nomes de personagem:
    -- ["Fulano","Beltrano"]. É o que torna "fulano está botando" conferível.
    -- JSONB e não tabela filha porque nada consulta dentro: a tela mostra a
    -- denúncia inteira.
    por_perto JSONB NOT NULL DEFAULT '[]'::jsonb,

    -- Quem tratou, e quando. NULL enquanto ninguém pegou.
    tratado_em  TIMESTAMPTZ,
    tratado_por BIGINT
);

-- A fila de atendimento: o que ainda não foi tratado, mais antigo primeiro —
-- quem esperou mais é quem tem que ser visto antes.
CREATE INDEX player_report_abertos_idx
    ON player_report (criado_em) WHERE tratado_em IS NULL;

-- Todas as denúncias de uma conta, para a página dela.
CREATE INDEX player_report_conta_idx ON player_report (conta_id, criado_em DESC);

-- A varredura do prazo.
CREATE INDEX player_report_expira_idx ON player_report (expira_em);
