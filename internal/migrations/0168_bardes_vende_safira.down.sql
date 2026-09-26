-- Tira a Safira e o Pacote de Safiras da loja do Bardes. Tira também se eles já
-- estivessem lá antes da 0168: a migração não guarda de onde veio cada vaga.
DELETE FROM npc_shop_item
WHERE item_index IN (697, 4131)
  AND npc_id IN (SELECT id FROM npc_definition WHERE lower(btrim(template_name)) = 'bardes');

UPDATE npc_config_meta SET version = version + 1 WHERE id = TRUE;
