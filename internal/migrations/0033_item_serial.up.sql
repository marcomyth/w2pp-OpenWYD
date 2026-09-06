-- 0033_item_serial — dar identidade ao item que vale a pena.
--
-- Até aqui item não tinha identidade nenhuma: é um índice mais três pares de
-- efeito, e duas unidades iguais são indistinguíveis. O censo (0032) responde
-- "apareceram três a mais esta semana". Não responde "esta aqui é a cópia".
-- Isto responde.
--
-- O SERIAL NÃO CABE NO ITEM DO JOGO. O STRUCT_ITEM original é 8 bytes —
-- sIndex mais três pares (Basedef.h:500) — e não sobra byte nenhum. Nos
-- servidores antigos, marcar item significava gastar os três espaços de efeito
-- com o número, e aí a espada +11 deixava de ser +11.
--
-- Aqui não, e o precedente já existe no próprio código: ExpiresAt é um carimbo
-- Unix por item, guardado no banco, que o cliente nunca vê (o comentário em
-- handler/item.go diz isso com todas as letras). serial anda pelo mesmo
-- caminho. Custo em espaço de efeito: zero. Item refinado guarda o refino E o
-- número.
--
-- ZERO SIGNIFICA "SEM MARCA", não "marca número zero". Item que já existia
-- antes desta migração nasce zero e ganha número no primeiro save do dono. Por
-- isso o índice é PARCIAL: a esmagadora maioria das linhas é zero e não
-- interessa a ninguém.
--
-- E o que isto NÃO faz: não impede dupe. Quem impede é salvar na hora da troca
-- (0025 e o commit que ligou SaveCharacterAsync). Isto torna a cópia PROVÁVEL,
-- que é coisa diferente e que nada mais neste plano alcança: duas linhas com o
-- mesmo serial não são indício, são prova.

ALTER TABLE item ADD COLUMN serial BIGINT NOT NULL DEFAULT 0;

-- A consulta que paga tudo: dois itens com o mesmo número.
CREATE INDEX item_serial_idx ON item (serial) WHERE serial <> 0;

-- O contador. Uma linha só, garantida pelo CHECK: quem inventar uma segunda
-- linha esbarra na chave primária.
--
-- Não é SEQUENCE de propósito. O tmServer não pode consultar o banco dentro do
-- laço do jogo — o laço é dono único de todo o estado e não bloqueia —, então
-- ele reserva um BLOCO de números de uma vez e distribui de memória. Uma
-- sequence daria um número por chamada, que é exatamente o que não dá para
-- fazer aqui.
CREATE TABLE item_serial_seq (
    unico   BOOLEAN PRIMARY KEY DEFAULT true CHECK (unico),
    proximo BIGINT NOT NULL DEFAULT 1
);

INSERT INTO item_serial_seq (unico, proximo) VALUES (true, 1);
