-- O filtro de drop das fadas (Painel de Drop - Fadas, pedido da dona em 02/10/2026).
--
-- Com a Fada Azul ou a Vermelha vestida e o filtro LIGADO, o saque de monstro que
-- não está nesta lista é descartado antes de entrar na mochila
-- (handler/fadas_filtro.go). A lista é do PERSONAGEM, e não da conta: o que um
-- guerreiro guarda não é o que um mago guarda.
--
-- Uma linha por personagem, com a lista inteira num vetor: ela é pequena (teto de
-- 60 itens, o tamanho da mochila) e é sempre lida e gravada de uma vez.
-- Personagem sem linha é filtro desligado e lista vazia.
CREATE TABLE IF NOT EXISTS fada_filtro (
    character_id BIGINT PRIMARY KEY REFERENCES character(id) ON DELETE CASCADE,
    ligado       BOOLEAN NOT NULL DEFAULT FALSE,
    itens        SMALLINT[] NOT NULL DEFAULT '{}',
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    -- Ligado com a lista vazia descartaria TODO o saque. O tmServer recusa; isto
    -- é a mesma regra para quem gravar por outro caminho.
    CONSTRAINT fada_filtro_ligado_tem_itens CHECK (NOT ligado OR cardinality(itens) > 0),
    CONSTRAINT fada_filtro_teto CHECK (cardinality(itens) <= 60)
);
