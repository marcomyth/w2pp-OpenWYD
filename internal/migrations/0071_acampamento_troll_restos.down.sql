DELETE FROM drop_rule WHERE mob IN ('ATroll_Insano', 'ATroll_Cacador', 'ATroll_Mago', 'ATroll_Caos', 'ATroll_Enigma') AND item IN (419, 420);
UPDATE drop_rule_meta SET version = version + 1 WHERE id = TRUE;
