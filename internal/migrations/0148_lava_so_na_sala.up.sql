-- 0148_lava_so_na_sala — o saque da 0142 passa a valer só na sala da foto.
--
-- Correção do Marco em 25/09/2026: o pedido era a sala das fotos, em (972,3994),
-- e a 0142 mexeu no Golem de Pedra e no Anf Ninja do 2º andar inteiro, porque a
-- Mesa de Drops vale por template.
--
-- A sala é x 964-985, y 3984-4005, entre as paredes do HeightMap. Os quatro blocos
-- dela (2296 a 2299) passam a usar cópias dos templates, Golem_Lava e
-- Anf_Ninja_Lava: o nome de dentro do arquivo continua o mesmo, então o jogador vê
-- o mesmo Golem de Pedra e o mesmo Anf Ninja. O saque da 0142 muda para as cópias.
-- O resto do andar volta ao que era antes da 0142: o template, com o Sem Sela da
-- 0091 (0,33% o N e 0,29% o B).
DELETE FROM drop_rule WHERE
    (mob = 'Anf_Ninja' AND item IN (2392, 2394, 2395, 2441, 4019, 4018, 4026, 412, 697, 852, 882, 883, 1182, 1189, 1192, 1194, 1330, 1345, 1477, 1489, 1627, 1636, 1639, 1709, 4039, 4040)) OR
    (mob = 'Golem_de_Pedra' AND item IN (2392, 2394, 2395, 2441, 4019, 4018, 4026, 412, 413, 419, 420, 426, 454, 909, 934, 1471, 1621, 4039, 4040, 4041));

INSERT INTO drop_rule (mob, item, chance) VALUES
    ('Anf_Ninja',      2396, 33),       -- Âmago de Cav s/Sela N
    ('Anf_Ninja',      2401, 29),       -- Âmago de Ca s/Sela B
    ('Golem_de_Pedra', 2396, 33),       -- Âmago de Cav s/Sela N
    ('Golem_de_Pedra', 2401, 29)        -- Âmago de Ca s/Sela B
ON CONFLICT (mob, item) DO UPDATE SET chance = EXCLUDED.chance, updated_at = now();

INSERT INTO drop_rule (mob, item, chance) VALUES
    ('Anf_Ninja_Lava', 2392,  50),  -- Âmago de Lobo
    ('Anf_Ninja_Lava', 2394,  50),  -- Âmago de Urso
    ('Anf_Ninja_Lava', 2395,  40),  -- Âmago de Dente de Sabre
    ('Anf_Ninja_Lava', 2441,  30),  -- Diamante
    ('Anf_Ninja_Lava', 4019,  50),  -- Classe D
    ('Anf_Ninja_Lava', 4018,  40),  -- Classe C
    ('Anf_Ninja_Lava', 4026,  30),  -- Moeda de Prata(1Mi)
    ('Anf_Ninja_Lava', 2396,  20),  -- Âmago de Cav s/Sela N
    ('Anf_Ninja_Lava', 2401,  15),  -- Âmago de Ca s/Sela B
    ('Anf_Ninja_Lava',  412,   0),  -- Poeira de Oriharucon
    ('Anf_Ninja_Lava',  697,   0),  -- Safira
    ('Anf_Ninja_Lava',  852,   0),  -- Lança da Corrupção
    ('Anf_Ninja_Lava',  882,   0),  -- Shamir
    ('Anf_Ninja_Lava',  883,   0),  -- Faca Astaroth
    ('Anf_Ninja_Lava', 1182,   0),  -- Calça Dourada(N)
    ('Anf_Ninja_Lava', 1189,   0),  -- Botas Douradas(M)
    ('Anf_Ninja_Lava', 1192,   0),  -- Elmo Anão(M)
    ('Anf_Ninja_Lava', 1194,   0),  -- Armadura Anã(N)
    ('Anf_Ninja_Lava', 1330,   0),  -- Túnica Conjuradora(M)
    ('Anf_Ninja_Lava', 1345,   0),  -- Túnica de Mytril(M)
    ('Anf_Ninja_Lava', 1477,   0),  -- Elmo Aeon(M)
    ('Anf_Ninja_Lava', 1489,   0),  -- Botas Aeon(M)
    ('Anf_Ninja_Lava', 1627,   0),  -- Chapéu da Natureza(M)
    ('Anf_Ninja_Lava', 1636,   0),  -- Luvas da Natureza(M)
    ('Anf_Ninja_Lava', 1639,   0),  -- Botas da Natureza(M)
    ('Anf_Ninja_Lava', 1709,   0),  -- Aegis
    ('Anf_Ninja_Lava', 4039,   0),  -- Colheita do Jardineiro
    ('Anf_Ninja_Lava', 4040,   0),  -- Cura do Batedor
    ('Golem_Lava',     2392,  50),  -- Âmago de Lobo
    ('Golem_Lava',     2394,  50),  -- Âmago de Urso
    ('Golem_Lava',     2395,  40),  -- Âmago de Dente de Sabre
    ('Golem_Lava',     2441,  30),  -- Diamante
    ('Golem_Lava',     4019,  50),  -- Classe D
    ('Golem_Lava',     4018,  40),  -- Classe C
    ('Golem_Lava',     4026,  30),  -- Moeda de Prata(1Mi)
    ('Golem_Lava',     2396,  20),  -- Âmago de Cav s/Sela N
    ('Golem_Lava',     2401,  15),  -- Âmago de Ca s/Sela B
    ('Golem_Lava',      412,   0),  -- Poeira de Oriharucon
    ('Golem_Lava',      413,   0),  -- Poeira de Lactolerium
    ('Golem_Lava',      419,   0),  -- Resto de Oriharucon
    ('Golem_Lava',      420,   0),  -- Resto de Lactolerium
    ('Golem_Lava',      426,   0),  -- Cristal VI
    ('Golem_Lava',      454,   0),  -- Moeda de Ouro 1
    ('Golem_Lava',      909,   0),  -- Lâmina Dupla
    ('Golem_Lava',      934,   0),  -- Grande Machado
    ('Golem_Lava',     1471,   0),  -- Manoplas de Osso(M)
    ('Golem_Lava',     1621,   0),  -- Braçadeira de Combate(M)
    ('Golem_Lava',     4039,   0),  -- Colheita do Jardineiro
    ('Golem_Lava',     4040,   0),  -- Cura do Batedor
    ('Golem_Lava',     4041,   0)   -- Mana do Batedor
ON CONFLICT (mob, item) DO UPDATE SET chance = EXCLUDED.chance, updated_at = now();

UPDATE drop_rule_meta SET version = version + 1 WHERE id = TRUE;
