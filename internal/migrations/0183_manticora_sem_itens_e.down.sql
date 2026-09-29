-- Devolve à Mantícora comum as armas E e os elmos do set E do template.
DELETE FROM drop_rule
WHERE mob = 'Manticora' AND item IN (3551, 3571, 3591, 1225, 1360, 1510, 1660);

UPDATE drop_rule_meta SET version = version + 1 WHERE id = TRUE;
