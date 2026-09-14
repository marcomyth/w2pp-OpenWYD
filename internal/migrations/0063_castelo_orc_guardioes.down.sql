DELETE FROM drop_rule WHERE mob IN ('COrc_Sentinela', 'COrc_Capitao') AND item IN (2396, 2401, 4027, 3173);
DELETE FROM drop_rule WHERE mob = 'COrc_Mago';
UPDATE drop_rule_meta SET version = version + 1 WHERE id = TRUE;
