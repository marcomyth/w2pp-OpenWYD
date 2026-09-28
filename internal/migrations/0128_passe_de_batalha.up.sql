-- 0128_passe_de_batalha — o nível do passe, na CONTA.
--
-- O passe é uma moldura em volta do nome do jogador, que o cliente desenha. Ele não
-- dá vantagem nenhuma por enquanto: esta migração guarda só QUAL é o nível, e o
-- resto — quantas pergas por dia, quantos dias — é decisão da dona do servidor e
-- ainda não existe. Não deduza vantagem daqui.
--
-- NA CONTA E NÃO NO PERSONAGEM, e isso é uma escolha do desenho: a pessoa compra o
-- passe uma vez e ele vale para todos os personagens dela. Casa com o caminho que
-- vem depois — o site dando o passe sozinho quando a doação for confirmada, e a
-- doação é por conta.
--
-- 0 = sem passe; 1 a 4 = as quatro molduras. O CHECK está aqui ALÉM da conferência
-- no código, e não em vez dela: o código protege a porta que ele conhece, e o banco
-- protege a linha de qualquer porta — inclusive de um UPDATE à mão numa madrugada.
--
-- Um valor fora da faixa faria o cliente não desenhar nada (ele limita a 0..4 e cai
-- para zero), o que é o melhor dos estragos possíveis. Ainda assim é errado, e é
-- barato impedir.
ALTER TABLE account
    ADD COLUMN IF NOT EXISTS passe_nivel SMALLINT NOT NULL DEFAULT 0;

ALTER TABLE account
    DROP CONSTRAINT IF EXISTS account_passe_nivel_faixa;

ALTER TABLE account
    ADD CONSTRAINT account_passe_nivel_faixa
    CHECK (passe_nivel BETWEEN 0 AND 4);
