-- Tira a trava global do WYD TOTO. Nenhum template o carrega, então na prática
-- ele continua sem cair e sem ser vendido até que alguém o ponha pelo painel.
DELETE FROM drop_rule WHERE mob = '*' AND item = 4147;

UPDATE drop_rule_meta SET version = version + 1 WHERE id = TRUE;
