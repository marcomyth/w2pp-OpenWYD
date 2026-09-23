-- 0108_wyd_toto_fora — o WYD TOTO (4147) não cai nem se compra.
--
-- O item não faz nada: o volátil 207 dele não tem tratamento no servidor, e usar
-- só devolve "não pode usar" (handler/item.go, rejectUnimplementedConsumable).
-- Decisão do Marco, 23/09/2026: tirar de drops e lojas, para ninguém receber
-- item inútil.
--
-- Nenhum template o carrega hoje — nem vitrine nem tabela de drop, nos 2014
-- arquivos —, então esta migração cuida do que o painel possa ter posto: as
-- vagas de loja e as regras da Mesa de Drops. A regra global ("*", chance 0)
-- é a mesma trava da 0086 e da 0088: segura o item mesmo que um template volte
-- a trazê-lo.
DELETE FROM npc_shop_item WHERE item_index = 4147;

UPDATE npc_config_meta SET version = version + 1 WHERE id = TRUE;

INSERT INTO drop_rule (mob, item, chance) VALUES ('*', 4147, 0)
ON CONFLICT (mob, item) DO UPDATE SET chance = 0, updated_at = now();

DELETE FROM drop_rule WHERE item = 4147 AND mob <> '*';

UPDATE drop_rule_meta SET version = version + 1 WHERE id = TRUE;
