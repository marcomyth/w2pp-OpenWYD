-- Devolve a 0142: o saque volta para o Golem de Pedra e o Anf Ninja de todo o
-- andar, e sai das cópias da sala. Os blocos e os templates voltam pelo git.
DELETE FROM drop_rule WHERE mob IN ('Golem_Lava', 'Anf_Ninja_Lava');

INSERT INTO drop_rule (mob, item, chance) VALUES
    ('Anf_Ninja',      2392,  50),  -- Âmago de Lobo
    ('Anf_Ninja',      2394,  50),  -- Âmago de Urso
    ('Anf_Ninja',      2395,  40),  -- Âmago de Dente de Sabre
    ('Anf_Ninja',      2441,  30),  -- Diamante
    ('Anf_Ninja',      4019,  50),  -- Classe D
    ('Anf_Ninja',      4018,  40),  -- Classe C
    ('Anf_Ninja',      4026,  30),  -- Moeda de Prata(1Mi)
    ('Anf_Ninja',      2396,  20),  -- Âmago de Cav s/Sela N
    ('Anf_Ninja',      2401,  15),  -- Âmago de Ca s/Sela B
    ('Anf_Ninja',       412,   0),  -- Poeira de Oriharucon
    ('Anf_Ninja',       697,   0),  -- Safira
    ('Anf_Ninja',       852,   0),  -- Lança da Corrupção
    ('Anf_Ninja',       882,   0),  -- Shamir
    ('Anf_Ninja',       883,   0),  -- Faca Astaroth
    ('Anf_Ninja',      1182,   0),  -- Calça Dourada(N)
    ('Anf_Ninja',      1189,   0),  -- Botas Douradas(M)
    ('Anf_Ninja',      1192,   0),  -- Elmo Anão(M)
    ('Anf_Ninja',      1194,   0),  -- Armadura Anã(N)
    ('Anf_Ninja',      1330,   0),  -- Túnica Conjuradora(M)
    ('Anf_Ninja',      1345,   0),  -- Túnica de Mytril(M)
    ('Anf_Ninja',      1477,   0),  -- Elmo Aeon(M)
    ('Anf_Ninja',      1489,   0),  -- Botas Aeon(M)
    ('Anf_Ninja',      1627,   0),  -- Chapéu da Natureza(M)
    ('Anf_Ninja',      1636,   0),  -- Luvas da Natureza(M)
    ('Anf_Ninja',      1639,   0),  -- Botas da Natureza(M)
    ('Anf_Ninja',      1709,   0),  -- Aegis
    ('Anf_Ninja',      4039,   0),  -- Colheita do Jardineiro
    ('Anf_Ninja',      4040,   0),  -- Cura do Batedor
    ('Golem_de_Pedra', 2392,  50),  -- Âmago de Lobo
    ('Golem_de_Pedra', 2394,  50),  -- Âmago de Urso
    ('Golem_de_Pedra', 2395,  40),  -- Âmago de Dente de Sabre
    ('Golem_de_Pedra', 2441,  30),  -- Diamante
    ('Golem_de_Pedra', 4019,  50),  -- Classe D
    ('Golem_de_Pedra', 4018,  40),  -- Classe C
    ('Golem_de_Pedra', 4026,  30),  -- Moeda de Prata(1Mi)
    ('Golem_de_Pedra', 2396,  20),  -- Âmago de Cav s/Sela N
    ('Golem_de_Pedra', 2401,  15),  -- Âmago de Ca s/Sela B
    ('Golem_de_Pedra',  412,   0),  -- Poeira de Oriharucon
    ('Golem_de_Pedra',  413,   0),  -- Poeira de Lactolerium
    ('Golem_de_Pedra',  419,   0),  -- Resto de Oriharucon
    ('Golem_de_Pedra',  420,   0),  -- Resto de Lactolerium
    ('Golem_de_Pedra',  426,   0),  -- Cristal VI
    ('Golem_de_Pedra',  454,   0),  -- Moeda de Ouro 1
    ('Golem_de_Pedra',  909,   0),  -- Lâmina Dupla
    ('Golem_de_Pedra',  934,   0),  -- Grande Machado
    ('Golem_de_Pedra', 1471,   0),  -- Manoplas de Osso(M)
    ('Golem_de_Pedra', 1621,   0),  -- Braçadeira de Combate(M)
    ('Golem_de_Pedra', 4039,   0),  -- Colheita do Jardineiro
    ('Golem_de_Pedra', 4040,   0),  -- Cura do Batedor
    ('Golem_de_Pedra', 4041,   0)   -- Mana do Batedor
ON CONFLICT (mob, item) DO UPDATE SET chance = EXCLUDED.chance, updated_at = now();

UPDATE drop_rule_meta SET version = version + 1 WHERE id = TRUE;
