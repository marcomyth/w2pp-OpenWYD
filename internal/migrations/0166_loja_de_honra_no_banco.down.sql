-- Tira o estoque da Loja de Honra das vagas do God of War. Só faz sentido junto
-- com a volta do código que tinha o estoque em handler/loja_de_honra.go.
DELETE FROM npc_shop_item
WHERE npc_id IN (SELECT id FROM npc_definition WHERE lower(btrim(template_name)) = 'god_of_war');

UPDATE npc_config_meta SET version = version + 1 WHERE id = TRUE;
