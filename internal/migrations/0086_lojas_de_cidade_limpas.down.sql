-- Tira a regra global que zera a Refinação Abençoada; o drop dela volta dos
-- templates de monstro, que esta migração não tocou.
--
-- Os itens retirados das lojas e as quantidades NÃO são restaurados aqui: eles
-- vinham dos templates, e com as vagas livres e os arquivos de
-- Release/TMsrv/run/npc/ revertidos pelo git o dbServer ressemeia a prateleira
-- antiga no boot seguinte.
DELETE FROM drop_rule WHERE mob = '*' AND item = 3338;

UPDATE drop_rule_meta SET version = version + 1 WHERE id = TRUE;

UPDATE npc_config_meta SET version = version + 1 WHERE id = TRUE;
