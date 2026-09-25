-- 0156_guer_caveira_amagos_e_restos — o Guer. Caveira da Dungeon passa a soltar
-- os âmagos de Lobo, Dragão Menor e Dente de Sabre e o Resto de Oriharucon.
--
-- Pedido do Marco em 25/09/2026, com print em (741,3742) no 2º piso. Os drops são
-- SOMADOS ("além dos drops que eles já possuem"): o que o template solta continua,
-- o Âmago de Urso da 0091 (0,5%) fica, e a Pedra do Esqueleto segue a 0% (0140).
-- Nenhum dos quatro itens está no Carry do template, então nenhum slot é pulado.
--
--   Âmago de Lobo 2392 e de Dragão Menor 2393   0,5% (50)
--   Âmago de Dente de Sabre 2395                0,4% (40)
--   Resto de Oriharucon 419                     1%   (100)
--
-- O Marco não deu número. São a régua da Dungeon: 0,5% é a escada de âmagos da
-- 0091, 0,4% é o Dente de Sabre do salão de lava (0142_lava_golem_e_ninja), e 1% é
-- o Resto dos monstros de campo e das salas do 1º andar (0143). Pelo viés do
-- sorteio da Mesa (droprule/vies_test.go) cada um paga 22,1% a mais. Ajustável na
-- tela /drops.
--
-- A escolta da Boss Hidra Dourada (Guer_Caveira_Escolta, 0149) recebe o mesmo: a
-- 0149 copiou a Mesa do Guer_Caveira do momento, e sem esta linha a escolta — a
-- mais forte dos dois — ficaria sem os drops que o Guer_Caveira comum ganhou.
--
-- Volume: são ~100 Guer_Caveira nos andares 1 e 2, morrem com um golpe e voltam
-- em 12 a 24 s — foi por isso que a Pedra do Esqueleto saiu na 0140. Se o Resto
-- sair demais, é aqui que se corta. O Guer._Caveira_ (1310,316) não é da Dungeon
-- e fica de fora.
INSERT INTO drop_rule (mob, item, chance) VALUES
    ('Guer_Caveira',         2392,  50),  -- Âmago de Lobo
    ('Guer_Caveira',         2393,  50),  -- Âmago de Dragão Menor
    ('Guer_Caveira',         2395,  40),  -- Âmago de Dente de Sabre
    ('Guer_Caveira',          419, 100),  -- Resto de Oriharucon
    ('Guer_Caveira_Escolta', 2392,  50),  -- Âmago de Lobo
    ('Guer_Caveira_Escolta', 2393,  50),  -- Âmago de Dragão Menor
    ('Guer_Caveira_Escolta', 2395,  40),  -- Âmago de Dente de Sabre
    ('Guer_Caveira_Escolta',  419, 100)   -- Resto de Oriharucon
ON CONFLICT (mob, item) DO UPDATE SET chance = EXCLUDED.chance, updated_at = now();

UPDATE drop_rule_meta SET version = version + 1 WHERE id = TRUE;
