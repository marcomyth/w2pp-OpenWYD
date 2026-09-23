-- Tira a regra global que zera a Bolsa da Sorte. O drop por monstro volta dos
-- templates revertidos pelo git, e as lojas voltam a ser semeadas no boot.
DELETE FROM drop_rule WHERE mob = '*' AND item = 4104;

UPDATE drop_rule_meta SET version = version + 1 WHERE id = TRUE;

UPDATE donate_shop_item SET enabled = TRUE, updated_at = now() WHERE item_index = 4104;
UPDATE daily_reward_item SET enabled = TRUE, updated_at = now() WHERE item_index = 4104;

UPDATE npc_config_meta SET version = version + 1 WHERE id = TRUE;
