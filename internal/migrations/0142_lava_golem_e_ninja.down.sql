-- Devolve o que a 0142 mudou na Mesa: o Golem de Pedra e o Anf Ninja voltam ao
-- que o template diz, com o Sem Sela da 0091 (0,33% o N e 0,29% o B). A
-- quantidade, os chefes e os templates deles voltam pelo git.
DELETE FROM drop_rule WHERE
    (mob = 'Anf_Ninja' AND item IN (2392, 2394, 2395, 2441, 4019, 4018, 4026, 412, 697, 852, 882, 883, 1182, 1189, 1192, 1194, 1330, 1345, 1477, 1489, 1627, 1636, 1639, 1709, 4039, 4040)) OR
    (mob = 'Golem_de_Pedra' AND item IN (2392, 2394, 2395, 2441, 4019, 4018, 4026, 412, 413, 419, 420, 426, 454, 909, 934, 1471, 1621, 4039, 4040, 4041));

INSERT INTO drop_rule (mob, item, chance) VALUES
    ('Anf_Ninja',      2396, 33),       -- Âmago de Cav s/Sela N
    ('Anf_Ninja',      2401, 29),       -- Âmago de Ca s/Sela B
    ('Golem_de_Pedra', 2396, 33),       -- Âmago de Cav s/Sela N
    ('Golem_de_Pedra', 2401, 29)        -- Âmago de Ca s/Sela B
ON CONFLICT (mob, item) DO UPDATE SET chance = EXCLUDED.chance, updated_at = now();

UPDATE drop_rule_meta SET version = version + 1 WHERE id = TRUE;
