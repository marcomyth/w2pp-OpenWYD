-- Volta o que dá para voltar: as regras '*' saem e a vaga 19 da Evolução volta como
-- a 0006/0007 a deixaram (4142, quantidade 255). A chuva de itens, a loja de doação
-- e a recompensa diária NÃO voltam: não há como saber quais estavam ligadas. As
-- regras de monstro nomeado que a ida apagou também não voltam.
DELETE FROM drop_rule WHERE mob = '*' AND item IN (414, 4142, 5600);
UPDATE drop_rule_meta SET version = version + 1 WHERE id = TRUE;

INSERT INTO npc_shop_item (npc_id, slot, item_index, quantity, eff1, effv1, eff2, effv2, eff3, effv3)
    SELECT id, 19, 4142, 255, 0, 0, 0, 0, 0, 0 FROM npc_definition WHERE template_name = 'Evolucao'
    ON CONFLICT (npc_id, slot) DO NOTHING;
UPDATE npc_config_meta SET version = version + 1 WHERE id = TRUE;
