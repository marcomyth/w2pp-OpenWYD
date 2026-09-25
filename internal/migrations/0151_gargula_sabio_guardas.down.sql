-- Tira o saque dos guardas da Gárgula Sábio. O bloco, os templates e o código
-- voltam pelo git.
DELETE FROM drop_rule WHERE mob = 'Golem_Guarda';

UPDATE drop_rule_meta SET version = version + 1 WHERE id = TRUE;
