-- Devolve a Pedra de Adamantita à vaga 15 dos oito vendedores que a tinham (os
-- quatro de Armia e os quatro de Azran; as cópias "_" nunca a venderam). A
-- quantidade volta como 255 em Armia e 1 em Azran, como estava antes da 0079.
INSERT INTO npc_shop_item (npc_id, slot, item_index, quantity)
SELECT d.id, 15, 578, v.quantidade
FROM npc_definition d
JOIN (VALUES
    ('Ferreiro', 255::smallint), ('Rapein', 255::smallint),
    ('Arnod',    255::smallint), ('Rainy',  255::smallint),
    ('Ferreiro_Azran', 1::smallint), ('Rapein_Azran', 1::smallint),
    ('Arnod_Azran',    1::smallint), ('Rainy_Azran',  1::smallint)
) AS v(template, quantidade) ON v.template = d.template_name
ON CONFLICT (npc_id, slot) DO UPDATE SET
    item_index = 578, quantity = EXCLUDED.quantity,
    eff1 = 0, effv1 = 0, eff2 = 0, effv2 = 0, eff3 = 0, effv3 = 0,
    price_points = NULL;

UPDATE npc_config_meta SET version = version + 1 WHERE id = TRUE;
