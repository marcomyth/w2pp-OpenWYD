-- 0066_poeira_de_fada_fora — a Poeira de Fada sai de toda venda e todo drop.
--
-- Decisão de 15/09/2026: os itens 414 e 4142 (Poeira_de_Fada) e 5600
-- (Poeira_de_Fada_Avançada) saem de tudo o que o jogador alcança sozinho. As que
-- já estão com jogadores continuam valendo, e a equipe ainda pode dar pelo painel
-- e pelo /gm (compensação, suporte). Entregas pendentes na delivery_queue ficam:
-- já foram pagas.
--
-- Esta migração é metade do conserto. A outra metade são as casas zeradas nos
-- templates Release/TMsrv/run/npc/Evolucao (Carry[55]) e Nordic_Store___
-- (Carry[1]): o dbServer ressemeia a loja a partir dos templates em todo boot
-- (SeedNPCDefinitions, INSERT ... ON CONFLICT (npc_id, slot) DO NOTHING), então
-- só apagar aqui faria a vaga da Evolução voltar no boot seguinte.
--
-- Cada alteração só mexe em linha cujo item é uma das três poeiras.

-- 1. Venda: a loja da Evolução, e qualquer outra loja de NPC que as tenha.
--    A versão só sobe se alguma linha saiu, para o tmServer reler a loja.
WITH removidas AS (
    DELETE FROM npc_shop_item WHERE item_index IN (414, 4142, 5600) RETURNING 1
)
UPDATE npc_config_meta SET version = version + 1
WHERE id = TRUE AND EXISTS (SELECT 1 FROM removidas);

-- 2. Drop: a regra '*' a 0% tira o item de todo monstro, inclusive dos que ainda
--    não nascem (Demi_Lord, Lovey e Minion carregam o 4142): o kill pula as casas
--    do Carry que a Mesa de Drops governa (droprule.Table.Governs,
--    internal/droprule/droprule.go:111-118; mobkilled.go:108). Uma regra de
--    monstro nomeado passaria por cima da '*', então as nomeadas saem.
INSERT INTO drop_rule (mob, item, chance) VALUES
    ('*', 414, 0),
    ('*', 4142, 0),
    ('*', 5600, 0)
ON CONFLICT (mob, item) DO UPDATE SET chance = 0, updated_at = now();

DELETE FROM drop_rule WHERE item IN (414, 4142, 5600) AND mob <> '*';

UPDATE drop_rule_meta SET version = version + 1 WHERE id = TRUE;

-- 3. Chuva de itens do evento de mundo: desliga só se o item configurado for uma
--    das poeiras. A versão só sobe nesse caso.
WITH desligada AS (
    UPDATE world_event_config SET enabled = FALSE, updated_at = now()
    WHERE item_index IN (414, 4142, 5600) AND enabled
    RETURNING 1
)
UPDATE world_event_meta SET version = version + 1
WHERE id = TRUE AND EXISTS (SELECT 1 FROM desligada);

-- 4. Loja de doação e recompensa diária: tira da vitrine as linhas com as
--    poeiras, sem apagar o histórico de compras e resgates.
UPDATE donate_shop_item SET enabled = FALSE, updated_at = now()
WHERE item_index IN (414, 4142, 5600) AND enabled;

UPDATE daily_reward_item SET enabled = FALSE, updated_at = now()
WHERE item_index IN (414, 4142, 5600) AND enabled;
