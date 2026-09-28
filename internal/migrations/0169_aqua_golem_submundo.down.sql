-- Tira as regras da 0169: o Aqua Golem volta ao saque do template e da 0090.
DELETE FROM drop_rule
WHERE mob = 'Aqua_Golem' AND item IN (419, 420, 4042, 4019, 4026, 2307, 2312);

UPDATE drop_rule_meta SET version = version + 1 WHERE id = TRUE;
