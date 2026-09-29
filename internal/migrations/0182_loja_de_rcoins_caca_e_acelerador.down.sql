-- Tira da vitrine as sete ofertas que a 0182 inseriu, pela mesma chave
-- (item_index, title). O que já foi comprado vive na delivery_queue.
DELETE FROM donate_shop_item d
USING (VALUES
    (3432, 'Pedido de Caça (Armia) ×120'), (3433, 'Pedido de Caça (Dungeon) ×120'),
    (3434, 'Pedido de Caça (Submundo) ×120'), (3435, 'Pedido de Caça (Kult) ×120'),
    (3436, 'Pedido de Caça (Kefra) ×120'), (3437, 'Pedido de Caça (Nippleheim) ×120'),
    (3438, 'Acelerador de Nascimento')
) AS v(item_index, title)
WHERE d.item_index = v.item_index AND d.title = v.title;
