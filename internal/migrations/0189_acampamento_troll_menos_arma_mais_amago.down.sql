-- Volta a tropa e os Magos do Acampamento Troll ao que a 0064 dava.
DELETE FROM drop_rule
WHERE mob IN ('ATroll_Insano', 'ATroll_Cacador') AND item IN (2397, 2402);

INSERT INTO drop_rule (mob, item, chance) VALUES
    ('ATroll_Insano',   869,   50), ('ATroll_Cacador',  869,   50), ('ATroll_Mago',   869,   50),
    ('ATroll_Insano',   809,   50), ('ATroll_Cacador',  809,   50), ('ATroll_Mago',   809,   50),
    ('ATroll_Insano',   910,   50), ('ATroll_Cacador',  910,   50), ('ATroll_Mago',   910,   50),
    ('ATroll_Insano',   824,   50), ('ATroll_Cacador',  824,   50), ('ATroll_Mago',   824,   50),
    ('ATroll_Insano',   935,   50), ('ATroll_Cacador',  935,   50), ('ATroll_Mago',   935,   50),
    ('ATroll_Insano',   899,   50), ('ATroll_Cacador',  899,   50), ('ATroll_Mago',   899,   50),
    ('ATroll_Insano',   854,   50), ('ATroll_Cacador',  854,   50), ('ATroll_Mago',   854,   50),
    ('ATroll_Insano',   902,   50), ('ATroll_Cacador',  902,   50), ('ATroll_Mago',   902,   50),

    ('ATroll_Insano',  2396,  300), ('ATroll_Cacador', 2396,  300), ('ATroll_Mago',  2396,  900),
    ('ATroll_Insano',  2401,  200), ('ATroll_Cacador', 2401,  200), ('ATroll_Mago',  2401,  700),
    ('ATroll_Mago',    2397,  500),
    ('ATroll_Mago',    2402,  300)
ON CONFLICT (mob, item) DO UPDATE SET chance = EXCLUDED.chance, updated_at = now();

UPDATE drop_rule_meta SET version = version + 1 WHERE id = TRUE;
