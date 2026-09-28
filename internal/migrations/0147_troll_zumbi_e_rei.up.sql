-- 0147_troll_zumbi_e_rei — o Troll Zumbi do 1º andar da Dungeon passa a soltar
-- Âmago de Dente de Sabre, Moeda de 1 milhão e Restos de Oriharucon e de
-- Lactolerium; e o andar ganha o Rei Troll Zumbi, um mini chefe de hora em hora.
--
-- Pedido do Marco em 25/09/2026: "todos trolls zumbis também devem dropar âmagos
-- de Dente de Sabre em uma proporção legal, adicione também barra de 1KK com
-- probabilidade baixa e restos de ori e lac com probabilidade baixa também".
--
-- O Troll Zumbi só nasce no 1º andar (53 bichos com os 19 do mesmo commit), então
-- estas regras não vazam para outra área. O Âmago de Lobo a 0,50% da 0091 fica.
--
-- Chance ESCRITA e ENTREGUE: o sorteio da Mesa é rand()%10000 sobre um rand() do
-- MSVC que para em 32767, e toda chance abaixo de 27,68% sai 22,1% maior
-- (internal/droprule/vies_test.go). O que o jogador vê é a coluna da direita:
--
--   Âmago de Dente de Sabre 2395   150   1,83%   um a cada ~55 abates
--   Moeda de Prata (1Mi)    4026    20   0,24%   um a cada ~410 (a régua do Kaizen, 0101)
--   Resto de Oriharucon      419    50   0,61%   um a cada ~164
--   Resto de Lactolerium     420    30   0,37%   um a cada ~273
--
-- O Rei Troll Zumbi (template Rei_Troll_Zumbi, bloco 6150) não tem linha aqui: o
-- pacote dele é código (handler/rei_troll_zumbi.go) e o Carry do template é
-- vazio. Uma regra da Mesa para um item do pacote, gravada depois pelo painel,
-- tira esse item do pacote.
INSERT INTO drop_rule (mob, item, chance) VALUES
    ('Troll_Zumbi', 2395, 150),       -- Âmago de Dente de Sabre
    ('Troll_Zumbi', 4026,  20),       -- Moeda de Prata (1Mi)
    ('Troll_Zumbi',  419,  50),       -- Resto de Oriharucon
    ('Troll_Zumbi',  420,  30)        -- Resto de Lactolerium
ON CONFLICT (mob, item) DO UPDATE SET chance = EXCLUDED.chance, updated_at = now();

UPDATE drop_rule_meta SET version = version + 1 WHERE id = TRUE;
