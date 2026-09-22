-- Volta as poções a uma por compra, como a 0100 as pôs.
--
-- O preço por unidade é do código (handler/shop.go), não desta migração: com
-- quantidade 1 ele não tem efeito nenhum, então não há o que desfazer aqui.

UPDATE npc_shop_item
SET quantity = 1
WHERE item_index IN (404, 409)
  AND npc_id IN (SELECT id FROM npc_definition WHERE template_name IN ('Aki', 'Martin'));

UPDATE npc_config_meta SET version = version + 1 WHERE id = TRUE;
