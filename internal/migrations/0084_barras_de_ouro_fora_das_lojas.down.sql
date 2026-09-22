-- Devolve as barras às lojas de NPC a partir dos templates: com as vagas livres
-- e os nove arquivos de Release/TMsrv/run/npc/ revertidos pelo git, o dbServer
-- ressemeia a prateleira no boot seguinte. A vitrine de doação e a recompensa
-- diária voltam a ficar à mostra aqui.
UPDATE donate_shop_item SET enabled = TRUE, updated_at = now()
WHERE item_index IN (4010, 4011, 4026, 4027, 4028, 4029);

UPDATE daily_reward_item SET enabled = TRUE, updated_at = now()
WHERE item_index IN (4010, 4011, 4026, 4027, 4028, 4029);

UPDATE npc_config_meta SET version = version + 1 WHERE id = TRUE;
