-- Devolve ao Verme, ao Aeon e ao Adamant do Deserto as armas E e as peças do
-- set E do template.
DELETE FROM drop_rule
WHERE (mob = 'Verme_'         AND item IN (3576, 3556, 1226, 1361, 1511, 1661))
   OR (mob = 'Aeon_Tauron'    AND item IN (3582, 3556, 3561, 1227, 1362, 1512, 1662))
   OR (mob = 'Adamant_Tauron' AND item IN (3596, 3566, 3581, 1229, 1364, 1514, 1664));

UPDATE drop_rule_meta SET version = version + 1 WHERE id = TRUE;
