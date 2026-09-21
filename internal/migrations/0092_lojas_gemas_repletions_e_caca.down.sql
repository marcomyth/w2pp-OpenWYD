-- Sem desfazer de conteúdo: as vitrines de verdade são os templates de
-- Release/TMsrv/run/npc/, e voltar atrás é reverter o commit e reiniciar, como
-- nas 0086 e 0091. O bump de versão faz o tmServer reler a configuração, que é
-- o que um down desta família precisa entregar.
UPDATE npc_config_meta SET version = version + 1 WHERE id = TRUE;
