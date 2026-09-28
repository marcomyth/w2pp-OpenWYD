-- Tira o saque da sala do Golem de Fogo. Os blocos, os templates e o código
-- voltam pelo git.
DELETE FROM drop_rule WHERE mob IN ('Golem_Fogo_Lava', 'Gargula_Lava');

UPDATE drop_rule_meta SET version = version + 1 WHERE id = TRUE;
