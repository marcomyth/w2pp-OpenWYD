-- Desfaz a 0175: os brincos da sala do Golem de Fogo voltam aos 10 da 0152, e as
-- Gárgulas do Molar voltam ao saque do template (brincos, sem braceletes).
UPDATE drop_rule SET chance = 10, updated_at = now()
WHERE mob IN ('Golem_Fogo_Lava', 'Gargula_Lava') AND item IN (591, 592, 593, 594, 595);

DELETE FROM drop_rule
WHERE mob IN ('Gargula_Inf', 'Gargula_Servo') AND item IN (591, 592, 593, 594, 595, 507, 510, 511, 512, 513, 514);

UPDATE drop_rule_meta SET version = version + 1 WHERE id = TRUE;
