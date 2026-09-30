-- Volta as chances das Armas D do Deserto ao que a 0180 e a 0186 davam.
INSERT INTO drop_rule (mob, item, chance) VALUES
    ('Manticora',       870, 33),
    ('Manticora',       911, 33),
    ('Manticora',       810, 33),
    ('Manticora',       869, 9),
    ('Manticora',       910, 9),
    ('Manticora',       809, 9),

    ('Adamant_Tauron',  936, 33),
    ('Adamant_Tauron',  855, 33),
    ('Adamant_Tauron',  902, 33),
    ('Adamant_Tauron',  935, 9),
    ('Adamant_Tauron',  854, 9),
    ('Adamant_Tauron',  900, 9),

    ('Verme_',          885, 33),
    ('Verme_',          884, 18),
    ('Verme_',          825, 18),

    ('Aeon_Tauron',     900, 9),
    ('Aeon_Tauron',     825, 9),
    ('Aeon_Tauron',     840, 9),

    ('Taron_Assassino', 869, 8),
    ('Taron_Assassino', 910, 8),
    ('Taron_Assassino', 809, 8),
    ('Taron_Assassino', 935, 8),
    ('Taron_Assassino', 854, 8),
    ('Taron_Assassino', 902, 8),
    ('Taron_Assassino', 899, 8),
    ('Taron_Assassino', 824, 8),
    ('Taron_Assassino', 839, 8),
    ('Taron_Assassino', 884, 8)
ON CONFLICT (mob, item) DO UPDATE SET chance = EXCLUDED.chance, updated_at = now();

UPDATE drop_rule_meta SET version = version + 1 WHERE id = TRUE;
