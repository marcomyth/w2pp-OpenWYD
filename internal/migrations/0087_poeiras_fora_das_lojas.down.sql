-- As poeiras voltam às lojas a partir dos templates: com as vagas livres e os
-- oito arquivos de Release/TMsrv/run/npc/ revertidos pelo git, o dbServer
-- ressemeia a prateleira no boot seguinte.
UPDATE npc_config_meta SET version = version + 1 WHERE id = TRUE;
