-- 0185_loja_de_rcoins_tigre_de_fogo — o Tigre de Fogo de 3, 5 e 7 dias na Loja
-- de Rcoin, pedido pelo Marco em 29/09/2026: 110, 150 e 200 Rcoins.
--
-- Vai na aba Esferas (6), ao lado do Shire, do Thoroughbred e do Klazedale, que
-- são as outras montarias de loja com prazo. O item é o 3990, o mesmo dos
-- Pacotes do Apoiador (0123): +350 de dano, +50 de magia, 35% de absorção contra
-- monstros e +12% de XP (internal/mountbonus, temp e tempExtra).
--
-- O prazo vai em expires_days, como no Shire da 0160: a compra o transforma em
-- EF_WDAY na forma não iniciada, e ele só começa a correr quando a montaria é
-- equipada. O 3990 não tem duração no catálogo, então é esse efeito que decide.
--
-- Mesmas regras da 0160: descrição não vazia (o site esconde oferta sem ela) e
-- idempotente por (item_index, title).
INSERT INTO donate_shop_item
    (item_index, eff1, effv1, price, title, description, enabled, expires_days, category)
SELECT v.item_index, 0, 0, v.price, v.title, v.description, TRUE, v.expires_days, 6
FROM (VALUES
    (1, 3990, 110, 'Tigre de Fogo 3 dias', 'Montaria: +350 de dano, +50 de magia, 35% de absorção contra monstros e +12% de XP. O prazo começa quando você equipa.', 3),
    (2, 3990, 150, 'Tigre de Fogo 5 dias', 'Montaria: +350 de dano, +50 de magia, 35% de absorção contra monstros e +12% de XP. O prazo começa quando você equipa.', 5),
    (3, 3990, 200, 'Tigre de Fogo 7 dias', 'Montaria: +350 de dano, +50 de magia, 35% de absorção contra monstros e +12% de XP. O prazo começa quando você equipa.', 7)
) AS v(ordem, item_index, price, title, description, expires_days)
WHERE NOT EXISTS (
    SELECT 1 FROM donate_shop_item d
    WHERE d.item_index = v.item_index AND d.title = v.title
)
ORDER BY v.ordem;
