-- Desfaz o 0080. Os buffs que estavam correndo somem com a tabela: quem tinha
-- 30 dias pagos fica sem eles.
DROP INDEX IF EXISTS guild_buff_expires_idx;
DROP TABLE IF EXISTS guild_buff;
