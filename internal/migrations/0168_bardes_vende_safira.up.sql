-- 0168_bardes_vende_safira — o Bardes passa a vender a Safira e o Pacote de
-- Safiras (pedido de 26/09/2026).
--
--   item  nome                   qtd  preço (catálogo, em ouro)
--   697   Safira                 1    1.000.000
--   4131  Pacote_Safiras(10)     1    7.500.000
--
-- Os dois itens e não a Safira em pilha de dez: na loja de NPC o preço é por
-- COMPRA (handler/shop.go, unidadesCobradas), e dez Safiras numa vaga sairiam
-- pelo preço de uma. O Pacote é item próprio, que o cliente conhece e que o jogo
-- conta como dez Safiras onde aceita pagamento em Safiras (sapphirePaymentPlan,
-- mestrehab.go). Nada no jogo abre o Pacote em dez unidades soltas.
--
-- O preço é o do catálogo (Release/Common/ItemList.csv); esta migração não mexe
-- em item_price. Uma exceção de preço que já exista para esses itens continua
-- valendo, em toda loja.
--
-- É a volta dos dois itens a UMA loja: a 0086 ("limpeza para o lançamento") os
-- tirou de todas, junto com as gemas e as Jóias, e esvaziou os templates no mesmo
-- commit. O template do Bardes continua sem eles, então a seed do boot não os
-- recoloca em mais lugar nenhum. O Bardes já os vendia na seed original (0006,
-- vagas 7 e 8).
--
-- Por migração, e não pelo painel, porque o usuário do painel ainda não grava
-- loja (a #167 liberou só a leitura; a gravação é o PR reservado para a 0164).
--
-- Nas primeiras vagas LIVRES do Bardes, e não em vagas fixas: a loja que está no
-- ar pode ter sido editada pelo painel, e uma vaga fixa poderia cobrir um item.
-- Item que o Bardes já venda não entra de novo. A marca de "vaga esvaziada pelo
-- painel" (npc_shop_slot_cleared) da vaga usada sai junto: ela não diz mais nada
-- sobre uma vaga ocupada.
--
-- A linha do Bardes existe desde a 0006, então esta migração acha onde gravar
-- inclusive numa base nova.
WITH bardes AS (
    SELECT id FROM npc_definition WHERE lower(btrim(template_name)) = 'bardes'
),
livres AS (
    SELECT b.id AS npc_id, s.slot::smallint AS slot,
           row_number() OVER (PARTITION BY b.id ORDER BY s.slot) AS ordem
    FROM bardes b
    CROSS JOIN generate_series(0, 26) AS s(slot)
    WHERE NOT EXISTS (
        SELECT 1 FROM npc_shop_item i WHERE i.npc_id = b.id AND i.slot = s.slot)
),
novos AS (
    SELECT b.id AS npc_id, v.item_index,
           row_number() OVER (PARTITION BY b.id ORDER BY v.posicao) AS ordem
    FROM bardes b
    CROSS JOIN (VALUES (1, 697), (2, 4131)) AS v(posicao, item_index)
    WHERE NOT EXISTS (
        SELECT 1 FROM npc_shop_item i WHERE i.npc_id = b.id AND i.item_index = v.item_index)
),
gravados AS (
    INSERT INTO npc_shop_item (npc_id, slot, item_index, quantity)
    SELECT l.npc_id, l.slot, n.item_index, 1::smallint
    FROM livres l
    JOIN novos n ON n.npc_id = l.npc_id AND n.ordem = l.ordem
    RETURNING npc_id, slot
)
DELETE FROM npc_shop_slot_cleared c
USING gravados g
WHERE c.npc_id = g.npc_id AND c.slot = g.slot;

UPDATE npc_config_meta SET version = version + 1 WHERE id = TRUE;
