-- Desfaz o 0081. As escalações somem com a tabela.
DROP INDEX IF EXISTS guild_city_squad_guild_idx;
DROP TABLE IF EXISTS guild_city_squad;
