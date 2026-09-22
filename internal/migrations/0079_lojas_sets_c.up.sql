-- 0079_lojas_sets_c — os quatro vendedores de armadura passam a vender os três
-- sets do degrau C da classe, na versão (N), sem add nenhum e sem refino.
--
-- O que estava à venda: o set FINAL de cada classe (Mortal, Templário, Corvo,
-- Legionário) +15, com Dano 30, AC 30, Magia 10, Crítico 70, Resist 18 ou
-- Special 18 gravados em cada peça. Era o melhor equipamento do jogo saindo de
-- NPC por ouro, com add melhor do que o que cai de chefe.
--
-- O que passa a estar: os três sets de nível ~150 da classe, limpos —
--   TK (Ferreiro): Dourado, Anão, Embutido
--   FM (Rapein):   Concha, Conjurador, Mytril
--   BM (Arnod):    Osso, Aeon, Elemental
--   HT (Rainy):    Combate, Natureza, Teia
-- cinco peças cada, uma linha da vitrine por set (o cliente desenha 5 por
-- linha), nas vagas 0-14. A Pedra de Adamantita que o vendedor já tinha é
-- preservada, com a quantidade dela, e desce para a vaga 15.
--
-- Vale para as doze definições: as quatro de Armia, as quatro de Azran e as
-- quatro cópias com sufixo "_", por template_name — um NPC criado no painel com
-- um desses templates é um vendedor de armadura da mesma classe e entra junto.
--
-- Esta migração é metade do conserto, como a 0070: o dbServer ressemeia a loja a
-- partir do template em todo boot (INSERT ... ON CONFLICT DO NOTHING), então os
-- doze arquivos de Release/TMsrv/run/npc/ mudam no mesmo commit. Por isso as
-- marcas de vaga esvaziada (0072) desses NPCs também saem: a prateleira inteira
-- foi redefinida aqui e no template, e uma marca velha deixaria uma vaga do
-- template nova sem ser semeada.

-- 1. Limpa a prateleira dos alvos, preservando UMA Pedra de Adamantita (a de
--    menor vaga) por vendedor — é o único item da loja que não é armadura.
DELETE FROM npc_shop_item s
WHERE s.npc_id IN (
        SELECT id FROM npc_definition
        WHERE template_name IN (
            'Ferreiro', 'Ferreiro_', 'Ferreiro_Azran',
            'Rapein',   'Rapein_',   'Rapein_Azran',
            'Arnod',    'Arnod_',    'Arnod_Azran',
            'Rainy',    'Rainy_',    'Rainy_Azran'))
  AND NOT (s.item_index = 578
           AND s.slot = (SELECT MIN(x.slot) FROM npc_shop_item x
                         WHERE x.npc_id = s.npc_id AND x.item_index = 578));

-- 2. A Adamantita desce para a vaga 15, logo abaixo dos três sets.
UPDATE npc_shop_item SET slot = 15
WHERE item_index = 578
  AND slot <> 15
  AND npc_id IN (
        SELECT id FROM npc_definition
        WHERE template_name IN (
            'Ferreiro', 'Ferreiro_', 'Ferreiro_Azran',
            'Rapein',   'Rapein_',   'Rapein_Azran',
            'Arnod',    'Arnod_',    'Arnod_Azran',
            'Rainy',    'Rainy_',    'Rainy_Azran'));

-- 3. Os três sets, nas vagas 0-14, sem efeito nenhum e quantidade 1.
INSERT INTO npc_shop_item (npc_id, slot, item_index, quantity)
SELECT d.id, p.slot::smallint, p.item_index, 1
FROM npc_definition d
JOIN (VALUES
    ('Ferreiro', 'TK'), ('Ferreiro_', 'TK'), ('Ferreiro_Azran', 'TK'),
    ('Rapein',   'FM'), ('Rapein_',   'FM'), ('Rapein_Azran',   'FM'),
    ('Arnod',    'BM'), ('Arnod_',    'BM'), ('Arnod_Azran',    'BM'),
    ('Rainy',    'HT'), ('Rainy_',    'HT'), ('Rainy_Azran',    'HT')
) AS v(template, classe) ON v.template = d.template_name
JOIN (VALUES
    ('TK',  0, 1176),    -- Elmo Dourado(N)
    ('TK',  1, 1179),    -- Armadura Dourada(N)
    ('TK',  2, 1182),    -- Calça Dourada(N)
    ('TK',  3, 1185),    -- Manoplas Douradas(N)
    ('TK',  4, 1188),    -- Botas Douradas(N)
    ('TK',  5, 1191),    -- Elmo Anão(N)
    ('TK',  6, 1194),    -- Armadura Anã(N)
    ('TK',  7, 1197),    -- Calça Anã(N)
    ('TK',  8, 1200),    -- Manopla Anã(N)
    ('TK',  9, 1203),    -- Botas Anã(N)
    ('TK', 10, 1206),    -- Elmo Embutido(N)
    ('TK', 11, 1209),    -- Armadura Embutida(N)
    ('TK', 12, 1212),    -- Calça Embutida(N)
    ('TK', 13, 1215),    -- Manoplas Embutidas(N)
    ('TK', 14, 1218),    -- Botas Embutidas(N)
    ('FM',  0, 1311),    -- Chapéu de Concha(N)
    ('FM',  1, 1314),    -- Túnica de Concha(N)
    ('FM',  2, 1317),    -- Calça de Concha(N)
    ('FM',  3, 1320),    -- Luvas de Concha(N)
    ('FM',  4, 1323),    -- Botas de Concha(N)
    ('FM',  5, 1326),    -- Chapéu Conjurador(N)
    ('FM',  6, 1329),    -- Túnica Conjuradora(N)
    ('FM',  7, 1332),    -- Calça Conjuradora(N)
    ('FM',  8, 1335),    -- Luvas Conjuradoras(N)
    ('FM',  9, 1338),    -- Botas Conjuradoras(N)
    ('FM', 10, 1341),    -- Chapéu de Mytril(N)
    ('FM', 11, 1344),    -- Túnica de Mytril(N)
    ('FM', 12, 1347),    -- Calça de Mytril(N)
    ('FM', 13, 1350),    -- Luvas de Mytril(N)
    ('FM', 14, 1353),    -- Botas de Mytril(N)
    ('BM',  0, 1461),    -- Elmo de Osso(N)
    ('BM',  1, 1464),    -- Armadura de Osso(N)
    ('BM',  2, 1467),    -- Calça de Osso(N)
    ('BM',  3, 1470),    -- Manoplas de Osso(N)
    ('BM',  4, 1473),    -- Botas de Osso(N)
    ('BM',  5, 1476),    -- Elmo Aeon(N)
    ('BM',  6, 1479),    -- Armadura Aeon(N)
    ('BM',  7, 1482),    -- Calça Aeon(N)
    ('BM',  8, 1485),    -- Manoplas Aeon(N)
    ('BM',  9, 1488),    -- Botas Aeon(N)
    ('BM', 10, 1491),    -- Elmo Elemental(N)
    ('BM', 11, 1494),    -- Armadura Elemental(N)
    ('BM', 12, 1497),    -- Calça Elemental(N)
    ('BM', 13, 1500),    -- Manoplas Elementais(N)
    ('BM', 14, 1503),    -- Botas Elementais(N)
    ('HT',  0, 1611),    -- Elmo de Combate(N)
    ('HT',  1, 1614),    -- Peitoral de Combate(N)
    ('HT',  2, 1617),    -- Calça De Combate(N)
    ('HT',  3, 1620),    -- Braçadeira de Combate(N)
    ('HT',  4, 1623),    -- Botas de Combate(N)
    ('HT',  5, 1626),    -- Chapéu da Natureza(N)
    ('HT',  6, 1629),    -- Peitoral da Natureza(N)
    ('HT',  7, 1632),    -- Calça da Natureza(N)
    ('HT',  8, 1635),    -- Luvas da Natureza(N)
    ('HT',  9, 1638),    -- Botas da Natureza(N)
    ('HT', 10, 1641),    -- Chapéu de Teia(N)
    ('HT', 11, 1644),    -- Peitoral de Teia(N)
    ('HT', 12, 1647),    -- Calça de Teia(N)
    ('HT', 13, 1650),    -- Braçadeira de Teia(N)
    ('HT', 14, 1653)     -- Botas de Teia(N)
) AS p(classe, slot, item_index) ON p.classe = v.classe
ON CONFLICT (npc_id, slot) DO UPDATE SET
    item_index = EXCLUDED.item_index, quantity = 1,
    eff1 = 0, effv1 = 0, eff2 = 0, effv2 = 0, eff3 = 0, effv3 = 0,
    price_points = NULL;

-- 4. Marcas de vaga esvaziada desses vendedores (ver o cabeçalho).
DELETE FROM npc_shop_slot_cleared
WHERE npc_id IN (
        SELECT id FROM npc_definition
        WHERE template_name IN (
            'Ferreiro', 'Ferreiro_', 'Ferreiro_Azran',
            'Rapein',   'Rapein_',   'Rapein_Azran',
            'Arnod',    'Arnod_',    'Arnod_Azran',
            'Rainy',    'Rainy_',    'Rainy_Azran'));

-- 5. Hot-reload: o tmServer só relê a configuração de NPC quando a versão sobe.
UPDATE npc_config_meta SET version = version + 1 WHERE id = TRUE;
