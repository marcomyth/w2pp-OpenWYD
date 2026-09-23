-- Desfaz a 0080: tira a regra '*' e as nomeadas destes itens, e os templates
-- voltam a decidir. As regras de painel apagadas na subida não voltam.
DELETE FROM drop_rule
WHERE item BETWEEN 575 AND 578 OR item BETWEEN 2171 AND 2250;

UPDATE drop_rule_meta SET version = version + 1 WHERE id = TRUE;
