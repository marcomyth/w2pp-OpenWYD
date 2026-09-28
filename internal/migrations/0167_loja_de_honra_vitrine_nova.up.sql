-- 0167_loja_de_honra_vitrine_nova — a vitrine da Loja de Honra muda (pedido de
-- 26/09/2026): os sete itens de antes ficam 30% mais baratos, e entram a
-- Repletion D em pilha de cinco, a Chave da Caçada Orc e o Ovo de Dente de Sabre.
--
-- Roda logo depois da 0166, que passou o estoque do código para as vagas do God
-- of War. Daqui para frente o estoque se muda pelo painel, sem migração e sem
-- reiniciar; esta é só a vitrine com que a loja volta do reinício.
--
-- A régua continua a de 25/09 (dias de barraca aberta 20 h por dia: 240 pontos
-- sem fada, 560 com a Fada Azul). Os preços antigos saem com 30% a menos, todos
-- redondos (100, 360, 480, 960, 1440 e 2400 são múltiplos de 10). As vagas vão em
-- ordem de preço, que é a ordem em que a janela as mostra.
--
--   vaga  item                               qtd  antes  agora  dias sem / com fada
--   0     413  Poeira de Lactolerium          1     100     70  0,3 / 0,1
--   1     3438 Acelerador de Nascimento       1     360    252  1,1 / 0,5
--   2     412  Poeira de Oriharucon           3     480    336  1,4 / 0,6
--   3     465  Chave da Caçada Orc            1       —    480  2   / 0,9   (nova)
--   4     4019 Repletion D (Classe D)         5       —    500  2,1 / 0,9   (nova)
--   5     3901 Fada Azul, 24 h                1     960    672  2,8 / 1,2
--   6     4140 Baú de Experiência             1    1440   1008  4,2 / 1,8
--   7     3173 Pergaminho da Água (N) LV1     3    1440   1008  4,2 / 1,8
--   8     3467 Bolsa do Andarilho             1    2400   1680  7   / 3
--   9     2305 Ovo de Dente de Sabre          1       —   3600  15  / 6,4   (nova)
--
-- A Chave da Caçada Orc é o item 465 (Chave_do_Rei_Orc no ItemList): a chave que
-- abre a corrida do Castelo Orc e o Acampamento Troll (handler/castelo_orc_run.go).
-- Ela cai nas arenas das Hidras e dos Elfos e no Deserto; na loja sai por um
-- preço de dois dias sem fada, para quem não achou a sua.
--
-- O Ovo é o item mais caro da loja de propósito: montaria de incubação 2, o
-- dobro de dias da Bolsa do Andarilho.
--
-- A Repletion D empilha (o Castelo Orc já a entrega em pacotes de 20) e sai em
-- pilha de cinco pelo preço da tabela, como as Poeiras de Oriharucon em três.
DELETE FROM npc_shop_item
WHERE npc_id IN (SELECT id FROM npc_definition WHERE lower(btrim(template_name)) = 'god_of_war');

INSERT INTO npc_shop_item (npc_id, slot, item_index, quantity, eff1, effv1, price_points)
SELECT d.id, v.slot, v.item_index, v.quantity, v.eff1, v.effv1, v.price_points
FROM npc_definition d
CROSS JOIN (VALUES
    (0::smallint, 413,  1::smallint, 0::smallint,   0::smallint,   70),
    (1::smallint, 3438, 1::smallint, 0::smallint,   0::smallint,  252),
    (2::smallint, 412,  3::smallint, 0::smallint,   0::smallint,  336),
    (3::smallint, 465,  1::smallint, 0::smallint,   0::smallint,  480),
    (4::smallint, 4019, 5::smallint, 0::smallint,   0::smallint,  500),
    (5::smallint, 3901, 1::smallint, 106::smallint, 1::smallint,  672),
    (6::smallint, 4140, 1::smallint, 0::smallint,   0::smallint, 1008),
    (7::smallint, 3173, 3::smallint, 0::smallint,   0::smallint, 1008),
    (8::smallint, 3467, 1::smallint, 0::smallint,   0::smallint, 1680),
    (9::smallint, 2305, 1::smallint, 0::smallint,   0::smallint, 3600)
) AS v(slot, item_index, quantity, eff1, effv1, price_points)
WHERE lower(btrim(d.template_name)) = 'god_of_war';

UPDATE npc_config_meta SET version = version + 1 WHERE id = TRUE;
