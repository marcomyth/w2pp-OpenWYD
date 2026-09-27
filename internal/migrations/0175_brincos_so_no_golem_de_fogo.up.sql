-- 0175_brincos_so_no_golem_de_fogo — os brincos (591-595) caíam demais: uma conta
-- encheu a bolsa em uma noite. Pedido do Marco em 27/09/2026.
--
-- Eles vinham de duas fontes:
--   1. a sala do Golem de Fogo do 2º andar (0152): Golem_Fogo_Lava e Gargula_Lava,
--      cada brinco a 10 (0,10%, 0,122% pelo viés), ~0,6% de brinco por morte;
--   2. o ninho da quest do Molar, em (817,4062): os TEMPLATES Gargula_Inf e
--      Gargula_Servo trazem os cinco brincos no Carry, quatro deles nos slots 4-7
--      (g_pDropRate 900 → 1/891 cada no nível 201, mais ainda com bônus de drop do
--      matador), ~0,45% de brinco por morte, fora da Mesa.
--
-- Depois disto os brincos ficam SÓ no Golem de Fogo da sala, a 3 cada (0,03%,
-- 0,0366% real pelo viés), 3,3× menos que os 10 da 0152 — o Marco pediu 10× (1)
-- e depois subiu para 0,03%. A sala inteira cai mais que isso, porque a Gárgula
-- dela deixa de soltar brinco.
-- A Gárgula da sala e as duas do Molar vão a 0%: a regra faz a Mesa pular o slot
-- do template, então o arquivo binário não precisa mudar.
--
-- No lugar dos brincos, as Gárgulas do Molar passam a soltar os seis braceletes
-- (507, 510-514) a 10 cada (0,10%, 0,122% real; ~0,73% de bracelete por morte).
-- A chance foi escolha minha; o pedido não a deu. O 515 "Bracelete" é o item
-- genérico sem efeito e fica de fora.
INSERT INTO drop_rule (mob, item, chance) VALUES
    ('Golem_Fogo_Lava',  591,  3),  -- Brinco de Athena
    ('Golem_Fogo_Lava',  592,  3),  -- Brinco de Titã
    ('Golem_Fogo_Lava',  593,  3),  -- Brinco de Zeus
    ('Golem_Fogo_Lava',  594,  3),  -- Brinco de Hecate
    ('Golem_Fogo_Lava',  595,  3),  -- Brinco de Hercules
    ('Gargula_Lava',     591,  0),  -- Brinco de Athena: sai
    ('Gargula_Lava',     592,  0),  -- Brinco de Titã: sai
    ('Gargula_Lava',     593,  0),  -- Brinco de Zeus: sai
    ('Gargula_Lava',     594,  0),  -- Brinco de Hecate: sai
    ('Gargula_Lava',     595,  0),  -- Brinco de Hercules: sai
    ('Gargula_Inf',      591,  0),  -- Brinco de Athena: sai
    ('Gargula_Inf',      592,  0),  -- Brinco de Titã: sai
    ('Gargula_Inf',      593,  0),  -- Brinco de Zeus: sai
    ('Gargula_Inf',      594,  0),  -- Brinco de Hecate: sai
    ('Gargula_Inf',      595,  0),  -- Brinco de Hercules: sai
    ('Gargula_Inf',      507, 10),  -- Bracelete de Hercules
    ('Gargula_Inf',      510, 10),  -- Bracelete de Athena
    ('Gargula_Inf',      511, 10),  -- Bracelete de Titã
    ('Gargula_Inf',      512, 10),  -- Bracelete de Gaia
    ('Gargula_Inf',      513, 10),  -- Bracelete de Zeus
    ('Gargula_Inf',      514, 10),  -- Bracelete de Hecate
    ('Gargula_Servo',    591,  0),  -- Brinco de Athena: sai
    ('Gargula_Servo',    592,  0),  -- Brinco de Titã: sai
    ('Gargula_Servo',    593,  0),  -- Brinco de Zeus: sai
    ('Gargula_Servo',    594,  0),  -- Brinco de Hecate: sai
    ('Gargula_Servo',    595,  0),  -- Brinco de Hercules: sai
    ('Gargula_Servo',    507, 10),  -- Bracelete de Hercules
    ('Gargula_Servo',    510, 10),  -- Bracelete de Athena
    ('Gargula_Servo',    511, 10),  -- Bracelete de Titã
    ('Gargula_Servo',    512, 10),  -- Bracelete de Gaia
    ('Gargula_Servo',    513, 10),  -- Bracelete de Zeus
    ('Gargula_Servo',    514, 10)   -- Bracelete de Hecate
ON CONFLICT (mob, item) DO UPDATE SET chance = EXCLUDED.chance, updated_at = now();

UPDATE drop_rule_meta SET version = version + 1 WHERE id = TRUE;
