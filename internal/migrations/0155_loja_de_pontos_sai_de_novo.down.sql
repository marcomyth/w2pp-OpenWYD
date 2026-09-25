-- Devolve o NPC Loja_de_Pontos ao mundo, sem estoque.
UPDATE npc_definition SET enabled = TRUE WHERE template_name = 'Loja_de_Pontos';

UPDATE npc_config_meta SET version = version + 1 WHERE id = TRUE;
