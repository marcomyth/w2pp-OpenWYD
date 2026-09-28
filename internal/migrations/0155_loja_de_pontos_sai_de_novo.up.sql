-- 0155_loja_de_pontos_sai_de_novo — desfaz a 0150: o NPC Loja_de_Pontos sai de
-- Armia outra vez, sem estoque (pedido de 25/09/2026).
--
-- A 0150 foi um engano. A loja onde se gastam os pontos de lojinha é a Loja de
-- Honra, o God of War com o painel próprio (handler/loja_de_honra.go), e a
-- vitrine pedida foi para lá, no código. A 0150 tinha reativado o NPC antigo que
-- a 0096 tirou do mundo, e o jogador via duas lojas de pontos lado a lado.
--
-- Desativa, não apaga, pela razão de sempre (0083, 0096): o slug carrega a
-- posição do bloco no NPCGener. O template continua vazio desde a 0150, então a
-- seed do boot não tem o que recolocar nas vagas que isto esvazia.
UPDATE npc_definition SET enabled = FALSE WHERE template_name = 'Loja_de_Pontos';

DELETE FROM npc_shop_item
WHERE npc_id IN (SELECT id FROM npc_definition WHERE template_name = 'Loja_de_Pontos');

UPDATE npc_config_meta SET version = version + 1 WHERE id = TRUE;
