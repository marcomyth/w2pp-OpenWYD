-- Tira a Loja de Pontos do mundo de novo, sem estoque. As armas Seladas da
-- vitrine antiga não voltam: elas viviam no template, que a 0150 esvaziou.
DELETE FROM npc_shop_item
WHERE npc_id IN (SELECT id FROM npc_definition WHERE template_name = 'Loja_de_Pontos');

UPDATE npc_definition SET enabled = FALSE WHERE template_name = 'Loja_de_Pontos';

UPDATE npc_config_meta SET version = version + 1 WHERE id = TRUE;
