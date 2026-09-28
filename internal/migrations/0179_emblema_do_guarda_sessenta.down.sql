-- Volta o Emblema do Guarda ao que era antes da 0179: as três regras da Mesa ao
-- número antigo (Argos e Argos_Errante com o valor que o painel tinha em
-- 28/09/2026), e os quatro templates de volta às vagas do Carry.
INSERT INTO drop_rule (mob, item, chance) VALUES
    ('Aqua_Golem',    4042, 100),
    ('Argos',         4042,  50),
    ('Argos_Errante', 4042,  50)
ON CONFLICT (mob, item) DO UPDATE SET chance = EXCLUDED.chance, updated_at = now();

DELETE FROM drop_rule
WHERE item = 4042
  AND mob IN ('Elfo_Negro_Abj', 'Cav._Elfo_Negro', 'CH_Troll_Ghoul', 'Troll_Ghoul');

UPDATE drop_rule_meta SET version = version + 1 WHERE id = TRUE;
