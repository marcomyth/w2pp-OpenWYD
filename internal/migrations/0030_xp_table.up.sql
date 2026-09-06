-- 0030_xp_table — a Mesa de XP: as quebras de experiência por zona e evolução.
--
-- O legado escolhe a tabela de divisores pelo bloco de 128 tiles onde a morte
-- acontece (MobKilled.cpp:443-1425), e não por um ajuste de mapa. São sete
-- ramos — o campo geral mais as seis masmorras instanciadas — e cada um tem
-- três tabelas, uma por evolução. Aqui guardamos SÓ o que um moderador mudou:
-- a ausência de linha significa "usa o legado", que é o padrão e o que o
-- servidor roda quando o dbServer não responde.
--
-- Por que uma linha por (zona, evolução) e não por corte: um corte sozinho não
-- quer dizer nada. A tabela é uma cadeia de `senão se`, e trocar um degrau no
-- meio muda o significado dos de baixo. Então a unidade de edição — e a unidade
-- de histórico — é a tabela inteira, guardada como JSONB na ordem em que o
-- moderador a escreveu.
--
-- cuts NULL e cuts '[]' são coisas diferentes, e a diferença importa: NULL é
-- "não mexi nessa tabela, use a do legado"; '[]' é "essa tabela não tem
-- nenhum corte", que é uma configuração legítima — o Pesadelo Normal já vem
-- assim de fábrica para Mortal e Arch.
--
-- rate_percent é alavanca nossa, não do legado: multiplica o valor final, DEPOIS
-- de toda a conta antiga, para que "200%" queira dizer exatamente o dobro do que
-- o jogo pagaria. 100 é o neutro.
--
-- O histórico não mora aqui: quem edita é o painel, e o painel já tem
-- admin_audit_log, que é append-only no banco (migração 0022). A ação
-- SET_XP_RULE guarda a tabela antiga e a nova no old_value/new_value, que é
-- exatamente o histórico que se quer poder ler depois.

CREATE TABLE xp_rule (
    zone          SMALLINT NOT NULL CHECK (zone BETWEEN 0 AND 6),
    tier          SMALLINT NOT NULL CHECK (tier IN (1, 2, 3)),
    rate_percent  INTEGER  NOT NULL DEFAULT 100 CHECK (rate_percent BETWEEN 0 AND 100000),
    cuts          JSONB,
    updated_by    BIGINT REFERENCES account(id) ON DELETE SET NULL,
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (zone, tier)
);

-- A versão é o que o tmServer compara para saber se precisa reler, no mesmo
-- formato de world_event_meta.
CREATE TABLE xp_rule_meta (
    id      BOOLEAN PRIMARY KEY DEFAULT TRUE CHECK (id),
    version BIGINT NOT NULL DEFAULT 0
);
INSERT INTO xp_rule_meta (id, version) VALUES (TRUE, 0);
