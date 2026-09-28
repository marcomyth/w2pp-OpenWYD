-- Devolve a Mesa ao que era antes da 0143: o Âmago de Lobo a 0,5% da 0091 fica
-- nos três, e o resto sai. A população volta pelo NPCGener.txt revertido no git.
DELETE FROM drop_rule WHERE mob IN ('Caveira', 'Urso_Zumbi', 'Arq_Caveira')
  AND item IN (2393, 2394, 419);

UPDATE drop_rule SET chance = 50, updated_at = now()
WHERE mob IN ('Caveira', 'Urso_Zumbi', 'Arq_Caveira') AND item = 2392;

UPDATE drop_rule_meta SET version = version + 1 WHERE id = TRUE;
