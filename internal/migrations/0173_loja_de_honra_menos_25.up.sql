-- 0173_loja_de_honra_menos_25 — a Loja de Honra fica 25% mais barata (pedido do
-- Marco, 26/09/2026), por cima da vitrine da 0171 (a da 0167 com os três Círculos).
--
-- Todo preço em pontos sai com 25% a menos, arredondado para baixo quando cai em
-- meio ponto (só a Poeira de Lactolerium: 52,5 vira 52). Vagas, itens e
-- quantidades ficam os da 0171, e a ordem por preço continua a mesma.
--
--   vaga  item                               qtd  antes  agora
--   0     413  Poeira de Lactolerium            1     70     52
--   1     448  Círculo Divino Puro (1)          1    100     75
--   2     449  Círculo Divino Puro (2)          1    100     75
--   3     450  Círculo Divino Puro (3)          1    100     75
--   4     3438 Acelerador de Nascimento         1    252    189
--   5     412  Poeira de Oriharucon             3    336    252
--   6     465  Chave da Caçada Orc              1    480    360
--   7     4019 Repletion D (Classe D)           5    500    375
--   8     3901 Fada Azul, 24 h                  1    672    504
--   9     4140 Baú de Experiência               1   1008    756
--   10    3173 Pergaminho da Água (N) LV1       3   1008    756
--   11    3467 Bolsa do Andarilho               1   1680   1260
--   12    2305 Ovo de Dente de Sabre            1   3600   2700
--
-- Como a 0171, reescreve a vitrine inteira: a loja lê o estoque destas vagas, e o
-- painel ainda não grava loja.
DELETE FROM npc_shop_item
WHERE npc_id IN (SELECT id FROM npc_definition WHERE lower(btrim(template_name)) = 'god_of_war');

INSERT INTO npc_shop_item (npc_id, slot, item_index, quantity, eff1, effv1, price_points)
SELECT d.id, v.slot, v.item_index, v.quantity, v.eff1, v.effv1, v.price_points
FROM npc_definition d
CROSS JOIN (VALUES
    (0::smallint, 413,  1::smallint, 0::smallint, 0::smallint,   52),
    (1::smallint, 448,  1::smallint, 0::smallint, 0::smallint,   75),
    (2::smallint, 449,  1::smallint, 0::smallint, 0::smallint,   75),
    (3::smallint, 450,  1::smallint, 0::smallint, 0::smallint,   75),
    (4::smallint, 3438, 1::smallint, 0::smallint, 0::smallint,  189),
    (5::smallint, 412,  3::smallint, 0::smallint, 0::smallint,  252),
    (6::smallint, 465,  1::smallint, 0::smallint, 0::smallint,  360),
    (7::smallint, 4019, 5::smallint, 0::smallint, 0::smallint,  375),
    (8::smallint, 3901, 1::smallint, 106::smallint, 1::smallint,  504),
    (9::smallint, 4140, 1::smallint, 0::smallint, 0::smallint,  756),
    (10::smallint, 3173, 3::smallint, 0::smallint, 0::smallint,  756),
    (11::smallint, 3467, 1::smallint, 0::smallint, 0::smallint, 1260),
    (12::smallint, 2305, 1::smallint, 0::smallint, 0::smallint, 2700)
) AS v(slot, item_index, quantity, eff1, effv1, price_points)
WHERE lower(btrim(d.template_name)) = 'god_of_war';

UPDATE npc_config_meta SET version = version + 1 WHERE id = TRUE;
