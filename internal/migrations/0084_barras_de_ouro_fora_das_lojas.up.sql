-- 0084_barras_de_ouro_fora_das_lojas — as barras que viram ouro saem de toda
-- venda (limpeza para o lançamento).
--
-- São seis itens que o jogador compra e usa para receber ouro: Moeda de Prata
-- 1Mi (4026) e 5Mi (4027), Barra de Prata 10Mi (4028), 50Mi (4029), 100Mi (4010)
-- e 1Bi (4011). Nove NPCs as vendiam — Gold, Bardes, Etc, Outros, h23horas,
-- Redmirom, Redmiron e os dois Imp_Inferno.
--
-- Só a VENDA, por decisão: os monstros que as soltam continuam soltando (o drop
-- é onde o ouro deve ser ganho), e quem já tem a barra na bolsa continua podendo
-- usar. A Mesa de Drops segue ajustando a raridade delas.
--
-- Como sempre, as duas metades: os nove templates de Release/TMsrv/run/npc/
-- perdem a vaga no mesmo commit, senão o dbServer as recoloca no boot seguinte.
-- Nos templates de MONSTRO as vagas de drop ficam intactas.
DELETE FROM npc_shop_item WHERE item_index IN (4010, 4011, 4026, 4027, 4028, 4029);

-- Loja de doação e recompensa diária: tira da vitrine sem apagar o histórico.
UPDATE donate_shop_item SET enabled = FALSE, updated_at = now()
WHERE item_index IN (4010, 4011, 4026, 4027, 4028, 4029) AND enabled;

UPDATE daily_reward_item SET enabled = FALSE, updated_at = now()
WHERE item_index IN (4010, 4011, 4026, 4027, 4028, 4029) AND enabled;

UPDATE npc_config_meta SET version = version + 1 WHERE id = TRUE;
