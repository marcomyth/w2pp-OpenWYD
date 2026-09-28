-- Volta a mesa do Golem_Lava aos números da 0148.
INSERT INTO drop_rule (mob, item, chance) VALUES
    ('Golem_Lava', 2392, 50),
    ('Golem_Lava', 2394, 50),
    ('Golem_Lava', 2395, 40),
    ('Golem_Lava', 2441, 30),
    ('Golem_Lava', 4019, 50),
    ('Golem_Lava', 4018, 40),
    ('Golem_Lava', 4026, 30),
    ('Golem_Lava', 2396, 20),
    ('Golem_Lava', 2401, 15)
ON CONFLICT (mob, item) DO UPDATE SET chance = EXCLUDED.chance, updated_at = now();

UPDATE drop_rule_meta SET version = version + 1 WHERE id = TRUE;
