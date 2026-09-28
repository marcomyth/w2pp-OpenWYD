-- Tira as quatro regras do Troll Zumbi. O Rei Troll Zumbi, o bloco 6150 e os
-- blocos de caça 6151-6160 voltam pelo git.
DELETE FROM drop_rule WHERE mob = 'Troll_Zumbi' AND item IN (2395, 4026, 419, 420);

UPDATE drop_rule_meta SET version = version + 1 WHERE id = TRUE;
