-- 0182_loja_de_rcoins_caca_e_acelerador — sete ofertas novas na Loja de Rcoin,
-- pedidas pelo Marco em 29/09/2026.
--
-- Consumíveis (aba 1): os seis Pedidos de Caça (3432-3437: Armia, Dungeon,
-- Submundo, Kult, Kefra e Nippleheim, o gelo), 120 unidades por 12 Rcoins cada.
-- Cada uso leva a um dos dez pontos do mapa, escolhido na janela do item, e gasta
-- uma (useHuntingScroll, tmserver handler/item.go). Os Pedidos não entram na lista
-- de dividir (internal/pilha): chegam numa pilha só de 120 com EF_AMOUNT, como a
-- Trombeta Mágica ×120 e a Ração de Cavalo ×120 da 0160, e 120 é o teto de pilha
-- do jogo (pilha.MaxPorPilha).
--
-- Montaria (aba 3): o Acelerador de Nascimento (3438), uma unidade por 10 Rcoins.
-- Arrastado sobre um ovo, dá +1 de refino sem sorteio, mesmo com a espera da
-- incubação correndo (useBirthAccelerator, handler/birthaccelerator.go).
--
-- Com seis a mais, Consumíveis passa de 17 para 23 ofertas e ganha uma segunda
-- página na janela do jogo (RcoinPorPagina = 20).
--
-- Mesmas regras da 0160: descrição não vazia (o site esconde oferta sem ela) e
-- idempotente por (item_index, title), para não duplicar o que a equipe já tenha
-- cadastrado igual no painel.
INSERT INTO donate_shop_item
    (item_index, eff1, effv1, price, title, description, enabled, expires_days, category)
SELECT v.item_index, v.eff1, v.effv1, v.price, v.title, v.description, TRUE, 0, v.category
FROM (VALUES
    (1, 3432, 61, 120, 12, 'Pedido de Caça (Armia) ×120',      'Leva você a um dos dez pontos de caça de Armia, escolhido na janela do item. Cada viagem gasta um.', 1),
    (2, 3433, 61, 120, 12, 'Pedido de Caça (Dungeon) ×120',    'Leva você a um dos dez pontos de caça da Dungeon, escolhido na janela do item. Cada viagem gasta um.', 1),
    (3, 3434, 61, 120, 12, 'Pedido de Caça (Submundo) ×120',   'Leva você a um dos dez pontos de caça do Submundo, escolhido na janela do item. Cada viagem gasta um.', 1),
    (4, 3435, 61, 120, 12, 'Pedido de Caça (Kult) ×120',       'Leva você a um dos dez pontos de caça de Kult, escolhido na janela do item. Cada viagem gasta um.', 1),
    (5, 3436, 61, 120, 12, 'Pedido de Caça (Kefra) ×120',      'Leva você a um dos dez pontos de caça do Kefra, escolhido na janela do item. Cada viagem gasta um.', 1),
    (6, 3437, 61, 120, 12, 'Pedido de Caça (Nippleheim) ×120', 'Leva você a um dos dez pontos de caça de Nippleheim, o gelo, escolhido na janela do item. Cada viagem gasta um.', 1),
    (7, 3438, 0,  0,   10, 'Acelerador de Nascimento',         'Arraste sobre um ovo de montaria: +1 de refino garantido, mesmo durante a espera da incubação.', 3)
) AS v(ordem, item_index, eff1, effv1, price, title, description, category)
WHERE NOT EXISTS (
    SELECT 1 FROM donate_shop_item d
    WHERE d.item_index = v.item_index AND d.title = v.title
)
ORDER BY v.ordem;
