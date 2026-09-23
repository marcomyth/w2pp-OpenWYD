-- 0088_lac_100_nao_cai — o Lactolerium 100 não cai de monstro nenhum.
--
-- O Lactolerium 100 (4141) é o refino garantido: força o âmago a 100% na Mesa
-- das Máquinas (refine.Amago). Como recompensa de abate ele desmonta a curva de
-- refino inteira, então a regra aqui é preventiva e definitiva.
--
-- A varredura do conteúdo não achou nenhuma fonte de drop: nenhum dos templates
-- de Release/TMsrv/run/npc/ o carrega no Carry, e o ItemDropList.txt não o cita.
-- O que a varredura NÃO alcança é a Mesa de Drops, que vive no banco e é editada
-- pelo painel — e é justamente ela que esta regra cobre: mob '*' com chance 0
-- tira o item de todo monstro, inclusive de uma regra que alguém venha a criar.
--
-- As nove lojas que o vendem (Aki, Acessorios3, Armas_Selado, h24horas, Loja de
-- Pontos, Merc. Elfo, Outros, Refinações e Store_Especial) NÃO são tocadas: o
-- pedido foi sobre drop, e a venda é uma decisão à parte.
INSERT INTO drop_rule (mob, item, chance) VALUES ('*', 4141, 0)
ON CONFLICT (mob, item) DO UPDATE SET chance = 0, updated_at = now();

DELETE FROM drop_rule WHERE item = 4141 AND mob <> '*';

UPDATE drop_rule_meta SET version = version + 1 WHERE id = TRUE;
