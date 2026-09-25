-- 0143_dungeon_salas_amagos_e_restos — as salas do começo do 1º andar da Dungeon
-- ficam cheias e passam a soltar os três âmagos e o Resto de Oriharucon.
--
-- Pedido do Marco em 25/09/2026, com prints das salas de grade sobre lava (x 127
-- a 260, y 3700 a 3860). No NPCGener.txt do mesmo commit, os blocos da área
-- ganham população: Urso Zumbi 57 -> 285 e Arq. Caveira 15 -> 75 (5x), Caveira
-- 66 -> 132 (2x). Cada bloco nasce como um grupo cheio do mesmo template.
--
-- Os drops são SOMADOS: o que o template já soltava continua ("o DropBase
-- continua, isso é só adicionar mais mobs e drops"). Nos três, a Mesa passa a
-- rolar:
--   Âmago de Lobo 2392, de Urso 2394 e de Dragão Menor 2393   0,5% cada (50)
--   Resto de Oriharucon 419                                   1% (100)
-- O 0,5% é a escada de âmagos da Dungeon (0091), que já era o do Âmago de Lobo
-- nos três. O Marco não deu número; estes são o ponto de partida, ajustável na
-- tela /drops.
--
-- Uma regra da Mesa vale para o template inteiro. A Caveira e o Urso Zumbi só
-- nascem nesta área; o Arq. Caveira também nasce no resto do 1º andar (9 blocos)
-- e no Pesadelo N (1 bloco de 5), e lá também passa a soltar os quatro.
--
-- O Urso Zumbi carrega Âmago de Urso no slot 61 (1 em 4.500): com a regra, o
-- slot é pulado e vale o 0,5% daqui.
INSERT INTO drop_rule (mob, item, chance) VALUES
    ('Caveira',     2392,  50),           -- Âmago de Lobo
    ('Caveira',     2393,  50),           -- Âmago de Dragão Menor
    ('Caveira',     2394,  50),           -- Âmago de Urso
    ('Caveira',      419, 100),           -- Resto de Oriharucon
    ('Urso_Zumbi',  2392,  50),           -- Âmago de Lobo
    ('Urso_Zumbi',  2393,  50),           -- Âmago de Dragão Menor
    ('Urso_Zumbi',  2394,  50),           -- Âmago de Urso
    ('Urso_Zumbi',   419, 100),           -- Resto de Oriharucon
    ('Arq_Caveira', 2392,  50),           -- Âmago de Lobo
    ('Arq_Caveira', 2393,  50),           -- Âmago de Dragão Menor
    ('Arq_Caveira', 2394,  50),           -- Âmago de Urso
    ('Arq_Caveira',  419, 100)            -- Resto de Oriharucon
ON CONFLICT (mob, item) DO UPDATE SET chance = EXCLUDED.chance, updated_at = now();

UPDATE drop_rule_meta SET version = version + 1 WHERE id = TRUE;
