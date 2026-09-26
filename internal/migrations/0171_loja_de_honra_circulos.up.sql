-- 0171_loja_de_honra_circulos — entram na Loja de Honra os três Círculos Divinos
-- Puros (448, 449 e 450), a 100 pontos cada (pedido do Marco, 26/09/2026).
--
-- Roda depois da 0167 e reescreve a vitrine inteira, como ela, porque as vagas
-- vão em ordem de preço, que é a ordem em que a janela as mostra: os Círculos
-- entram logo depois da Poeira de Lactolerium (70) e empurram o resto três vagas.
-- Os outros dez itens ficam com a vaga nova e o preço da 0167.
--
--   vaga  item                               qtd  pontos
--   0     413  Poeira de Lactolerium          1      70
--   1     448  Círculo Divino Puro (1)        1     100   (novo)
--   2     449  Círculo Divino Puro (2)        1     100   (novo)
--   3     450  Círculo Divino Puro (3)        1     100   (novo)
--   4     3438 Acelerador de Nascimento       1     252
--   5     412  Poeira de Oriharucon           3     336
--   6     465  Chave da Caçada Orc            1     480
--   7     4019 Repletion D (Classe D)         5     500
--   8     3901 Fada Azul, 24 h                1     672
--   9     4140 Baú de Experiência             1    1008
--   10    3173 Pergaminho da Água (N) LV1     3    1008
--   11    3467 Bolsa do Andarilho             1    1680
--   12    2305 Ovo de Dente de Sabre          1    3600
--
-- Os três Círculos Puros se distinguem pelo EF_INIT1/2/3 do catálogo; a vaga
-- não precisa de efeito próprio. Os Círculos Compostos (693-695) ficam de fora.
DELETE FROM npc_shop_item
WHERE npc_id IN (SELECT id FROM npc_definition WHERE lower(btrim(template_name)) = 'god_of_war');

INSERT INTO npc_shop_item (npc_id, slot, item_index, quantity, eff1, effv1, price_points)
SELECT d.id, v.slot, v.item_index, v.quantity, v.eff1, v.effv1, v.price_points
FROM npc_definition d
CROSS JOIN (VALUES
    (0::smallint,  413,  1::smallint, 0::smallint,   0::smallint,   70),
    (1::smallint,  448,  1::smallint, 0::smallint,   0::smallint,  100),
    (2::smallint,  449,  1::smallint, 0::smallint,   0::smallint,  100),
    (3::smallint,  450,  1::smallint, 0::smallint,   0::smallint,  100),
    (4::smallint,  3438, 1::smallint, 0::smallint,   0::smallint,  252),
    (5::smallint,  412,  3::smallint, 0::smallint,   0::smallint,  336),
    (6::smallint,  465,  1::smallint, 0::smallint,   0::smallint,  480),
    (7::smallint,  4019, 5::smallint, 0::smallint,   0::smallint,  500),
    (8::smallint,  3901, 1::smallint, 106::smallint, 1::smallint,  672),
    (9::smallint,  4140, 1::smallint, 0::smallint,   0::smallint, 1008),
    (10::smallint, 3173, 3::smallint, 0::smallint,   0::smallint, 1008),
    (11::smallint, 3467, 1::smallint, 0::smallint,   0::smallint, 1680),
    (12::smallint, 2305, 1::smallint, 0::smallint,   0::smallint, 3600)
) AS v(slot, item_index, quantity, eff1, effv1, price_points)
WHERE lower(btrim(d.template_name)) = 'god_of_war';

UPDATE npc_config_meta SET version = version + 1 WHERE id = TRUE;
