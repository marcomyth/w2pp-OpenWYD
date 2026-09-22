-- Tira as duas regras: sem elas o laço de drop volta a ler o slot 57 do template
-- da Aparição, que é a Moeda de Prata (5Mi) a 2,857%.
DELETE FROM drop_rule WHERE mob = 'Aparicao' AND item IN (4026, 4027);

UPDATE drop_rule_meta SET version = version + 1 WHERE id = TRUE;
