-- Tira da vitrine as três ofertas que a 0184 inseriu, pela mesma chave
-- (item_index, title). O que já foi comprado vive na delivery_queue.
DELETE FROM donate_shop_item d
USING (VALUES
    (3990, 'Tigre de Fogo 3 dias'), (3990, 'Tigre de Fogo 5 dias'), (3990, 'Tigre de Fogo 7 dias')
) AS v(item_index, title)
WHERE d.item_index = v.item_index AND d.title = v.title;
