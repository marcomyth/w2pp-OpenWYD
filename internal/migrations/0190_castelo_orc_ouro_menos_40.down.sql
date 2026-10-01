-- Volta as Moedas de Prata do Castelo Orc aos números da 0053 e da 0063.
INSERT INTO drop_rule (mob, item, chance) VALUES
    ('COrc_Cavaleiro', 4026, 1600), ('COrc_Arqueiro', 4026, 1600),
    ('COrc_MeioOrc',   4026, 1600), ('COrc_Mago',     4026, 1600),
    ('COrc_Sentinela', 4026, 1600), ('COrc_Capitao',  4026, 1600), ('COrc_Chefe', 4026, 1600),
    ('COrc_Sentinela', 4027, 1000), ('COrc_Capitao',  4027, 1000)
ON CONFLICT (mob, item) DO UPDATE SET chance = EXCLUDED.chance, updated_at = now();

UPDATE drop_rule_meta SET version = version + 1 WHERE id = TRUE;
