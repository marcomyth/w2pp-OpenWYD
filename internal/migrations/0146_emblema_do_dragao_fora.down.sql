-- Tira a regra global que zera o Emblema do Dragão: os templates voltam a
-- soltá-lo pelas próprias vagas.
DELETE FROM drop_rule WHERE mob = '*' AND item = 751;

UPDATE drop_rule_meta SET version = version + 1 WHERE id = TRUE;
