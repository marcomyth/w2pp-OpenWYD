-- Devolve as Poeiras dos Tauron às chances da 0109: 1% e 0,5%.
INSERT INTO drop_rule (mob, item, chance) VALUES
    ('Tauron',          412, 100), ('Tauron',          413, 50),
    ('Ladrao_Tauron',   412, 100), ('Ladrao_Tauron',   413, 50),
    ('Arqueiro_Tauron', 412, 100), ('Arqueiro_Tauron', 413, 50),
    ('Aeon_Tauron',     412, 100), ('Aeon_Tauron',     413, 50),
    ('Adamant_Tauron',  412, 100), ('Adamant_Tauron',  413, 50)
ON CONFLICT (mob, item) DO UPDATE SET chance = EXCLUDED.chance, updated_at = now();

UPDATE drop_rule_meta SET version = version + 1 WHERE id = TRUE;
