-- Tira da prateleira só o que a 0099 pôs: as duas poções de 500.
--
-- A Cura e a Mana do Batedor NÃO saem. Elas não foram acrescentadas aqui — são
-- da 0006, e a 0099 apenas as recolocou onde já deviam estar. Apagá-las na volta
-- desfaria a seed original, que é o contrário de reverter esta migração.

DELETE FROM npc_shop_item
WHERE item_index IN (404, 409)
  AND npc_id IN (SELECT id FROM npc_definition WHERE template_name IN ('Aki', 'Martin'));

UPDATE npc_config_meta SET version = version + 1 WHERE id = TRUE;
