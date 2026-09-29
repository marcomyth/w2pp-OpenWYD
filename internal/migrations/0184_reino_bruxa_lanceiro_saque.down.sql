-- Devolve à Bruxa e ao Lanceiro a mesa da 0074: o Andaluz a 200, sem Equipado,
-- Safira nem Classe D.
DELETE FROM drop_rule
WHERE mob IN ('Bruxa', 'Lanceiro', 'Bruxa_', 'Lanceiro_') AND item IN (2404, 2399, 697, 4019);

INSERT INTO drop_rule (mob, item, chance) VALUES
    ('Bruxa',     2405, 200), ('Lanceiro',  2405, 200),
    ('Bruxa_',    2400, 200), ('Lanceiro_', 2400, 200)
ON CONFLICT (mob, item) DO UPDATE SET chance = EXCLUDED.chance, updated_at = now();

UPDATE drop_rule_meta SET version = version + 1 WHERE id = TRUE;
