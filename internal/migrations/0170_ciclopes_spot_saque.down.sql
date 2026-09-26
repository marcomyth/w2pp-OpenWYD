-- Desfaz a 0170: tira as regras que ela criou. Uma regra que o painel tenha
-- afinado depois também sai, porque é a mesma linha.
DELETE FROM drop_rule
WHERE (lower(mob) = 'ciclope_cruel' AND item IN (4026, 419, 420, 2395, 2396, 2401, 2397, 2402))
   OR lower(mob) IN ('ciclope_cruel_spot', 'lanceiro_zakum_spot');

UPDATE drop_rule_meta SET version = version + 1 WHERE id = TRUE;
