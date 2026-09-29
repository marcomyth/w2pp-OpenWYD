-- Desfaz a 0186: o Verme, o Aeon e o Adamant voltam a soltar as armas E e o set
-- E do template, e os quatro perdem as Armas D e o Set D (A) que ela pôs na Mesa.
-- Os itens E da Mantícora continuam fora: são da 0183.
DELETE FROM drop_rule
WHERE (mob = 'Verme_'         AND item IN (3576, 3556, 1226, 1361, 1511, 1661, 884, 825, 1211, 1346, 1496, 1646))
   OR (mob = 'Manticora'      AND item IN (869, 910, 809, 1208, 1343, 1493, 1643))
   OR (mob = 'Aeon_Tauron'    AND item IN (3582, 3556, 3561, 1227, 1362, 1512, 1662, 900, 825, 840, 1214, 1349, 1499, 1649))
   OR (mob = 'Adamant_Tauron' AND item IN (3596, 3566, 3581, 1229, 1364, 1514, 1664, 935, 854, 900, 1220, 1355, 1505, 1655));

UPDATE drop_rule_meta SET version = version + 1 WHERE id = TRUE;
