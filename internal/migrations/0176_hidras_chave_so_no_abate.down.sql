-- Desfaz a 0176: a chave volta aos números da 0075, e o ovo e a moeda das Hidras
-- voltam ao template (a Dourada sem nenhum dos dois; a Imortal, a moeda pelos
-- slots 48, 49 e 62). O sorteio da chave na entrada volta pelo git.
UPDATE drop_rule SET chance = 50, updated_at = now()
WHERE item = 465 AND mob IN ('Hidra_Dourada', 'Mestre_Elfo');
UPDATE drop_rule SET chance = 20, updated_at = now()
WHERE item = 465 AND mob IN ('Hidra_Imortal', 'Servo_Elfo');

DELETE FROM drop_rule
WHERE mob IN ('Hidra_Dourada', 'Hidra_Imortal') AND item IN (2305, 4027);

UPDATE drop_rule_meta SET version = version + 1 WHERE id = TRUE;
