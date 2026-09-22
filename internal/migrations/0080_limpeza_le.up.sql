-- 0080_limpeza_le — os itens LE e as pedras que fazem LE só caem na Vila Amald e
-- no Kefra.
--
-- Decisão de 17/09/2026 (Reforma dos Drops, primeira etapa): as 80 peças (Le)
-- 2171-2250 e as pedras de Spinner, Beril, Tectita e Adamantita 575-578 (a pedra
-- transforma um set N/M/A no LE dele, useAdamantita) saem de todo monstro, e
-- voltam só nos monstros da Vila Amald (Karden, blocos 4050-4068 do NPCGener) e
-- do Kefra (Esquerda, Direita e Meio), com a chance que o template já dava.
--
-- A regra '*' a 0% tira o item de todo monstro, inclusive dos templates que ainda
-- não nascem; a regra de monstro nomeado vale por cima dela (droprule.Table).
--
-- A chance nomeada é a do template sem bônus de drop, com as casas repetidas do
-- Carry somadas, em centésimos de por cento e arredondada: 16,22 -> 16 e 10,50 ->
-- 11 na Vila Amald, 3,33 -> 3 e 1,01 -> 1 no Kefra. Diferença de uso: a Mesa não
-- passa pelo bônus de drop de quem mata, e o slot do template passava.
--
-- O Lich Batama também nasce no Vale Escondido, e a regra dele vale lá. O Tita
-- Berserker (Karden, fora da vila) perde a Adamantita.
--
-- Uma regra nomeada que o painel tiver gravado para estes itens sai: ou ela punha
-- LE fora das duas áreas, ou mudava o número decidido aqui. O down não a devolve.

INSERT INTO drop_rule (mob, item, chance)
SELECT '*', item, 0
FROM (
    SELECT generate_series(575, 578) AS item
    UNION ALL
    SELECT generate_series(2171, 2250)
) AS le
ON CONFLICT (mob, item) DO UPDATE SET chance = 0, updated_at = now();

DELETE FROM drop_rule
WHERE mob <> '*' AND (item BETWEEN 575 AND 578 OR item BETWEEN 2171 AND 2250);

INSERT INTO drop_rule (mob, item, chance) VALUES
    ('Templario_Amald', 2181, 16), ('Templario_Amald', 2182, 16), ('Templario_Amald', 2183, 16),
    ('Templario_Amald', 2184, 16), ('Templario_Amald', 2185, 16),
    ('Templario_Amald', 2186, 11), ('Templario_Amald', 2187, 11), ('Templario_Amald', 2188, 11),
    ('Templario_Amald', 2189, 11), ('Templario_Amald', 2190, 11),

    ('Mago_Amald', 2201, 16), ('Mago_Amald', 2202, 16), ('Mago_Amald', 2203, 16),
    ('Mago_Amald', 2204, 16), ('Mago_Amald', 2205, 16),
    ('Mago_Amald', 2206, 11), ('Mago_Amald', 2207, 11), ('Mago_Amald', 2208, 11),
    ('Mago_Amald', 2209, 11), ('Mago_Amald', 2210, 11),

    ('Shama_Amald', 2221, 16), ('Shama_Amald', 2222, 16), ('Shama_Amald', 2223, 16),
    ('Shama_Amald', 2224, 16), ('Shama_Amald', 2225, 16),
    ('Shama_Amald', 2226, 11), ('Shama_Amald', 2227, 11), ('Shama_Amald', 2228, 11),
    ('Shama_Amald', 2229, 11), ('Shama_Amald', 2230, 11),

    ('Ranger_Amald', 2241, 16), ('Ranger_Amald', 2242, 16), ('Ranger_Amald', 2243, 16),
    ('Ranger_Amald', 2244, 16), ('Ranger_Amald', 2245, 16),
    ('Ranger_Amald', 2246, 11), ('Ranger_Amald', 2247, 11), ('Ranger_Amald', 2248, 11),
    ('Ranger_Amald', 2249, 11), ('Ranger_Amald', 2250, 11),

    ('Batorero', 578, 4),

    ('Funer_Scyther', 2221, 5), ('Funer_Scyther', 2222, 5), ('Funer_Scyther', 2223, 5),
    ('Funer_Scyther', 2224, 5), ('Funer_Scyther', 2225, 5),
    ('Funer_Scyther', 576, 4),

    ('Funer_Seamer', 2181, 5), ('Funer_Seamer', 2182, 5), ('Funer_Seamer', 2183, 5),
    ('Funer_Seamer', 2184, 5), ('Funer_Seamer', 2185, 5),
    ('Funer_Seamer', 577, 2),

    ('Horizon_Cropper', 577, 3),

    ('Lich_Batama', 578, 3),

    ('Simio', 578, 1),

    ('Simio_Bleg', 578, 2),

    ('Xeno_Cropper', 577, 3),

    ('Funer_Momenter', 2201, 5), ('Funer_Momenter', 2202, 5), ('Funer_Momenter', 2203, 5),
    ('Funer_Momenter', 2204, 5), ('Funer_Momenter', 2205, 5),

    ('Funer_Sickler', 2187, 5), ('Funer_Sickler', 2209, 5), ('Funer_Sickler', 2226, 5),
    ('Funer_Sickler', 2227, 5), ('Funer_Sickler', 2249, 5),

    ('HorizonCropper', 577, 3),

    ('Aranha_Dourada', 2181, 5), ('Aranha_Dourada', 2182, 5), ('Aranha_Dourada', 2183, 5),
    ('Aranha_Dourada', 2184, 5), ('Aranha_Dourada', 2185, 5), ('Aranha_Dourada', 2221, 5),
    ('Aranha_Dourada', 2222, 5), ('Aranha_Dourada', 2223, 5), ('Aranha_Dourada', 2224, 5),
    ('Aranha_Dourada', 2225, 5),
    ('Aranha_Dourada', 577, 1),

    ('Aranha_Rubra', 2201, 5), ('Aranha_Rubra', 2202, 5), ('Aranha_Rubra', 2203, 5),
    ('Aranha_Rubra', 2204, 5), ('Aranha_Rubra', 2205, 5), ('Aranha_Rubra', 2241, 5),
    ('Aranha_Rubra', 2242, 5), ('Aranha_Rubra', 2243, 5), ('Aranha_Rubra', 2244, 5),
    ('Aranha_Rubra', 2245, 5),

    ('Rainha_Rubra', 577, 1),

    ('Serva_Rubra', 577, 1)
ON CONFLICT (mob, item) DO UPDATE SET chance = EXCLUDED.chance, updated_at = now();

UPDATE drop_rule_meta SET version = version + 1 WHERE id = TRUE;
