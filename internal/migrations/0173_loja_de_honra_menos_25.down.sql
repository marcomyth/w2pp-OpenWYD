-- Volta a vitrine da Loja de Honra à da 0171: os preços de antes do desconto.
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
