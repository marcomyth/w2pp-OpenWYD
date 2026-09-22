-- 0078_npc_shop_price_points — preço em PONTOS DE LOJINHA para um item de loja de NPC.
--
-- NULL (o padrão) = o item continua sendo vendido por OURO, pelo preço do catálogo
-- (ou pelo item_price do moderador). Um valor >= 0 troca a moeda daquele slot: o
-- tmServer debita a carteira de shop_points (0060) em vez do Coin do personagem.
--
-- Por SLOT e não pelo NPC inteiro de propósito: o mesmo vendedor pode ter uma
-- prateleira em ouro e outra em pontos, e o mesmo item pode custar ouro num NPC e
-- pontos em outro sem que um preço contamine o outro.
--
-- O cliente não sabe de nada disso: ele desenha o preço do ItemList.bin dele e
-- manda MSG_Buy sem consultar o ouro do jogador (desmontagem do wyd.exe,
-- 0x476d30-0x4775d6), então quem decide a moeda é o servidor.

ALTER TABLE npc_shop_item
    ADD COLUMN price_points INTEGER CHECK (price_points IS NULL OR price_points >= 0);
