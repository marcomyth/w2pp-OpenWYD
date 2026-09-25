-- 0150_loja_de_pontos_honra — a Loja de Pontos volta a Armia com sete itens
-- cobrados em pontos de lojinha (pedido de 25/09/2026).
--
-- A 0096 a tinha tirado do mundo com as armas Seladas, a Jóia da Escuridão e a
-- Pedra Lunar. Ela volta com outra vitrine, e o preço é medido em DIAS de
-- barraca aberta, 20 horas por dia: 240 pontos por dia sem fada (3 a cada 15
-- min) e 560 com a Fada Azul (7). A régua é a fada: quem ainda não tem uma
-- junta 4 dias para a de 24 horas.
--
--   vaga  item                           qtd  pontos  dias sem fada / com fada
--   0     413  Poeira de Lactolerium      1     100    ~8 h / ~3,5 h
--   1     3438 Acelerador de Nascimento   1     360    1,5  / 0,6
--   2     412  Poeira de Oriharucon       3     480    2    / 0,9
--   3     3901 Fada Azul, 24 h            1     960    4    / 1,7
--   4     4140 Baú de Experiência         1    1440    6    / 2,6
--   5     3173 Pergaminho da Água (N) LV1 3    1440    6    / 2,6
--   6     3467 Bolsa do Andarilho         1    2400    10   / 4,3
--
-- O preço é por COMPRA, não por unidade: a pilha de três sai pelo número da
-- tabela. A pilha vai na coluna quantity, que é onde o EF_AMOUNT (61) mora no
-- banco — normalizeShopQuantity tira o 61 dos efeitos e o guarda ali, e o
-- tmServer o devolve como 61 N no item entregue (shopEffects).
--
-- A fada é EXCEÇÃO consciente à 0107 ("nenhum comerciante vende fada"): esta
-- vende por pontos de tempo online, não por ouro, e só por 24 horas — o 106 1
-- (EF_WDAY) é o prazo, que começa a correr quando ela é equipada. Ela rende 320
-- pontos a mais por dia e custa 960, então não se paga sozinha.
--
-- AS DUAS METADES, como sempre: o template Release/TMsrv/run/npc/Loja_de_Pontos
-- fica SEM estoque no mesmo commit. Senão o dbServer recoloca as armas Seladas
-- nas vagas livres no boot seguinte (store.seedNPCShopRows). E vazio de propósito,
-- em vez de repetir estes sete itens: a seed não grava price_points, então um
-- item do template voltaria cobrado em OURO pelo preço do catálogo — a Bolsa do
-- Andarilho por mil de ouro.
UPDATE npc_definition SET enabled = TRUE WHERE template_name = 'Loja_de_Pontos';

DELETE FROM npc_shop_item
WHERE npc_id IN (SELECT id FROM npc_definition WHERE template_name = 'Loja_de_Pontos');

-- Vagas que o painel tenha esvaziado não dizem mais nada: o estoque inteiro é
-- este, e o template não tem o que recolocar.
DELETE FROM npc_shop_slot_cleared
WHERE npc_id IN (SELECT id FROM npc_definition WHERE template_name = 'Loja_de_Pontos');

INSERT INTO npc_shop_item (npc_id, slot, item_index, quantity, eff1, effv1, price_points)
SELECT d.id, v.slot, v.item_index, v.quantity, v.eff1, v.effv1, v.price_points
FROM npc_definition d
CROSS JOIN (VALUES
    (0::smallint, 413,  1::smallint, 0::smallint,   0::smallint,  100),
    (1::smallint, 3438, 1::smallint, 0::smallint,   0::smallint,  360),
    (2::smallint, 412,  3::smallint, 0::smallint,   0::smallint,  480),
    (3::smallint, 3901, 1::smallint, 106::smallint, 1::smallint,  960),
    (4::smallint, 4140, 1::smallint, 0::smallint,   0::smallint, 1440),
    (5::smallint, 3173, 3::smallint, 0::smallint,   0::smallint, 1440),
    (6::smallint, 3467, 1::smallint, 0::smallint,   0::smallint, 2400)
) AS v(slot, item_index, quantity, eff1, effv1, price_points)
WHERE d.template_name = 'Loja_de_Pontos';

UPDATE npc_config_meta SET version = version + 1 WHERE id = TRUE;
