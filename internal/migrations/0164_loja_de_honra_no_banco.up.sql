-- 0164_loja_de_honra_no_banco — o estoque da Loja de Honra sai do código e vai
-- para as vagas do God of War, editáveis no painel (pedido de 26/09/2026).
--
-- Até aqui os sete itens e os preços moravam em handler/loja_de_honra.go, e
-- mudar um preço exigia deploy — que reinicia o tmServer e derruba todo mundo.
-- Agora o tmServer lê o estoque das vagas do NPC, que a recarga das lojas de NPC
-- reescreve a cada 15 s. Cada vaga com price_points maior que zero é uma troca;
-- vaga em ouro ou a zero não aparece na Loja de Honra.
--
-- A vitrine é a mesma que estava no código, combinada em 25/09 (a 0150 a tinha
-- posto, por engano, no NPC Loja_de_Pontos — mesmos números, outro NPC):
--
--   vaga  item                           qtd  pontos
--   0     413  Poeira de Lactolerium      1     100
--   1     3438 Acelerador de Nascimento   1     360
--   2     412  Poeira de Oriharucon       3     480
--   3     3901 Fada Azul, 24 h            1     960   (106 1 = EF_WDAY, o prazo)
--   4     4140 Baú de Experiência         1    1440
--   5     3173 Pergaminho da Água (N) LV1 3    1440
--   6     3467 Bolsa do Andarilho         1    2400
--
-- A aba da janela (Armas, Set, Consumo) não é gravada: o tmServer a deduz da
-- casa de equipar do item. Os sete caem em Consumo, como no código.
--
-- O template Release/TMsrv/run/npc/God_of_War tem a mochila vazia, então a seed
-- do boot (store.seedNPCShopRows) não tem o que recolocar nestas vagas.
--
-- Numa base NOVA a linha do God of War ainda não existe quando esta migração
-- roda — ela nasce da reconciliação do catálogo, depois de store.Migrate (ver a
-- 0096) — e o INSERT não acha onde gravar: a Loja de Honra de uma base nova
-- começa vazia e é abastecida pelo painel. Produção já tem a linha.
--
-- Pelo template, e com a mesma folga de caixa e espaço que o tmServer usa para
-- reconhecer a loja (marcaLojaDeHonra): o slug carrega a posição do bloco no
-- NPCGener e muda quando o arquivo é editado.
DELETE FROM npc_shop_item
WHERE npc_id IN (SELECT id FROM npc_definition WHERE lower(btrim(template_name)) = 'god_of_war');

DELETE FROM npc_shop_slot_cleared
WHERE npc_id IN (SELECT id FROM npc_definition WHERE lower(btrim(template_name)) = 'god_of_war');

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
WHERE lower(btrim(d.template_name)) = 'god_of_war';

UPDATE npc_config_meta SET version = version + 1 WHERE id = TRUE;
