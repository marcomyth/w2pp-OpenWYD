-- As fadas voltam às lojas a partir dos templates: com as vagas livres e os
-- templates Fadas, Nordic_Store__, Utilidades e XTS_Store revertidos pelo git,
-- o dbServer ressemeia as prateleiras no boot seguinte.
UPDATE npc_config_meta SET version = version + 1 WHERE id = TRUE;
