-- 0076_combat_rule_garnet — a Garnet passa a absorver dano, com teto.
--
-- A Garnet (gema 3) não era lida por nada. No legado ela tira do golpe que
-- chega num jogador um valor FIXO, 40 por passo de refino acima de +9 por peça
-- (80 em Grade 8, CMob.cpp:873): uma montagem cheia +15 tira 2.640, e todo golpe
-- menor que isso vira 1 — imunidade a monstro comum e a quem não usa Esmeralda.
--
-- Decidido pelo Marco em 17/09/2026, pela simulação
-- (docs/balanceamento/garnet-esmeralda-2026-09-17.md), a regra A2:
--
--   1. a Garnet anula primeiro, por inteiro, a Esmeralda de quem bate
--      (até o total dela) — full contra full volta a ser a luta sem joia;
--   2. o que sobra dela tira no máximo garnet_pct% do resto do golpe.
--
-- Monstro não tem Esmeralda, então no PvE vale só o passo 2.
--
--   garnet_pct  0 a 100. 100 é o legado (subtrai o total inteiro);
--               0 deixa a Garnet só anulando a Esmeralda.
--
-- DEFAULT 20 é o padrão DECIDIDO, como nas 0046, 0052, 0054 e 0059. A faixa
-- repete internal/combatrule; combat_rule_range_test.go confere faixa e DEFAULT
-- contra o código. Lido AO VIVO, como o resto da linha.

ALTER TABLE combat_rule
    ADD COLUMN garnet_pct SMALLINT NOT NULL DEFAULT 20 CHECK (garnet_pct BETWEEN 0 AND 100);
