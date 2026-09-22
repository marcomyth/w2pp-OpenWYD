-- 0087_poeiras_fora_das_lojas — nenhum comerciante vende poeira.
--
-- Poeira de Oriharucon (412) e Poeira de Lactolerium (413) saem da vitrine de
-- todo NPC. Oito lojas as vendiam — Aki, Acessorios3, Etc, h20horas, Merc. Elfo,
-- Mercador Tekki, Refinações e Utilidades — várias em pacote de 240 ou 255, que
-- é o refino inteiro comprado de uma vez no balcão.
--
-- As poeiras de Fada (414, 4142 e a Avançada 5600) não estão em loja nenhuma, e
-- por isso não entram aqui: esta migração tira o que está à venda, não registra
-- proibição.
--
-- Como sempre, as duas metades: as oito lojas dos templates de
-- Release/TMsrv/run/npc/ perdem a vaga no mesmo commit, senão o dbServer as
-- recoloca no boot seguinte. O drop de monstro não é tocado — a poeira continua
-- caindo onde cai.
DELETE FROM npc_shop_item WHERE item_index IN (412, 413);

UPDATE npc_config_meta SET version = version + 1 WHERE id = TRUE;
