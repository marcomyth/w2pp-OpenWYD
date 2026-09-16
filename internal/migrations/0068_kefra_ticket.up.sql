-- 0068_kefra_ticket — as entradas do Hall do Kefra, por personagem.
--
-- O legado guarda isso em MobExtra.KefraTicket (Basedef.h:712): o NPC Sobrevivente
-- troca um Pergaminho_Selado (item 4127) por 100 entradas (_MSG_Quest.cpp:2597-2623)
-- e cada passagem pelo piso do Hall gasta uma (GetFunc.cpp:994-1004). Sem isso
-- ninguém entra na sala do chefe: o /kefra pousa em y 3884 e a área do Kefra
-- (2335-2394 x 3896-3954) é fechada a pé — medido no mapa de atributo.
--
-- Fica em coluna própria, como o resto da progressão que este port tirou do blob
-- de 552 bytes do MobExtra (a 0025 fez o mesmo com as entradas do Pesadelo). A
-- concessão e o gasto gravam na hora, porque o pergaminho é comprado.

ALTER TABLE character
    ADD COLUMN kefra_ticket INTEGER NOT NULL DEFAULT 0 CHECK (kefra_ticket >= 0);
