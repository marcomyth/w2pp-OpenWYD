-- Sem desfazer de conteúdo: a vitrine de verdade é o template de
-- Release/TMsrv/run/npc/Martin, e voltar atrás é reverter o commit e reiniciar,
-- como nas 0086, 0091, 0092 e 0094. O bump de versão faz o tmServer reler a
-- configuração, que é o que um down desta família precisa entregar.
UPDATE npc_config_meta SET version = version + 1 WHERE id = TRUE;
