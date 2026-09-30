-- A guilda dona da cidade nasce dentro da área da guild, como no jogo original.
--
-- A migração 0039 pôs no ponto de renascimento da guilda o spawn COMUM da cidade,
-- achando que o legado nunca preenchia GuildSpawnX/Y. Preenchia: a tabela fixa
-- g_pGuildZone em Source/Code/Basedef.cpp:56-60 traz os pontos, todos dentro da
-- área da guild de cada cidade (o bloco 0x20 do AttributeMap, guild_area.go).
-- Além disso, cada troca de imposto zerava o ponto (dbserver não repassava o
-- campo, corrigido em 29/09/2026), e na preview três cidades estavam em zero.
--
-- Os valores abaixo são os do legado, na ordem de zona: Armia, Azran, Erion,
-- Nippleheim, Noatum.
UPDATE guild_zone SET guild_spawn_x = 2088, guild_spawn_y = 2148 WHERE zone = 0;
UPDATE guild_zone SET guild_spawn_x = 2531, guild_spawn_y = 1700 WHERE zone = 1;
UPDATE guild_zone SET guild_spawn_x = 2460, guild_spawn_y = 1976 WHERE zone = 2;
UPDATE guild_zone SET guild_spawn_x = 3614, guild_spawn_y = 3124 WHERE zone = 3;
UPDATE guild_zone SET guild_spawn_x = 1066, guild_spawn_y = 1760 WHERE zone = 4;
