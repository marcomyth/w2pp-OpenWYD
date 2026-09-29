-- 0187_hold_da_morte_pvp — a dívida de experiência da morte em PvP (extra.Hold do
-- legado, MobKilled.cpp:3248-3263), pedido do Marco em 29/09/2026.
--
-- O legado nunca tira experiência de quem morre para outro jogador: soma a perda
-- aqui, e a experiência dos abates seguintes paga esta dívida antes de entrar na
-- barra (MobKilled.cpp:558-573). Por isso ninguém cai de nível, e o cliente mostra
-- o Hold ao lado da experiência. O port tirava direto da barra.
--
-- BIGINT porque o campo do legado é unsigned int: cabe até 4.294.967.295, que um
-- INTEGER não guarda.
ALTER TABLE character ADD COLUMN IF NOT EXISTS hold BIGINT NOT NULL DEFAULT 0
    CHECK (hold >= 0 AND hold <= 4294967295);
