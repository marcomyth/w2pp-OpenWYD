-- Tira a regra global que zera o Lactolerium 100. Nenhum template de monstro o
-- carrega, então na prática ele continua sem cair até que alguém crie uma regra.
DELETE FROM drop_rule WHERE mob = '*' AND item = 4141;

UPDATE drop_rule_meta SET version = version + 1 WHERE id = TRUE;
