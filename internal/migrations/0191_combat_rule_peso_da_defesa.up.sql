-- 0191_combat_rule_peso_da_defesa — quanto da defesa um golpe físico enfrenta
-- entre jogadores.
--
-- No legado a defesa do alvo é triplicada em PvP (_MSG_Attack.cpp:454-455) e o
-- BASE_GetDamage tira metade dela do golpe: dano − 1,5 × defesa. Em 12/09/2026 a
-- 0059 escalou o Ataque físico do jogador para 61% e deixou a defesa inteira.
-- Desde então um Ataque de 2.650 a 3.900 contra 2.100 a 3.500 de defesa não
-- deixa nada depois do desconto: medido em jogo em 01/10/2026, o BM tirava 1 do
-- TK e da HT, o TK só feria o BM no crítico e a HT tirava 40 a 100.
--
--   pvp_melee_armor_pct  0 a 150, em % da defesa. 150 é o legado (1,5 ×);
--                        100 é o peso que a skill já enfrenta (1 ×).
--
-- Só golpe físico de jogador em jogador. Golpe de monstro, golpe em monstro e
-- skill não leem esta coluna.
--
-- DEFAULT 150 é o legado, e NÃO muda nada no deploy: o número é escolhido no
-- painel (/rates/combate), que vale ao vivo. A faixa repete internal/combatrule;
-- combat_rule_range_test.go confere faixa e DEFAULT contra o código.

ALTER TABLE combat_rule
    ADD COLUMN pvp_melee_armor_pct SMALLINT NOT NULL DEFAULT 150 CHECK (pvp_melee_armor_pct BETWEEN 0 AND 150);
