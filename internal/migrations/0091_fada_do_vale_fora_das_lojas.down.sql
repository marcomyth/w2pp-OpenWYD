-- A fada volta à loja a partir do template: com a vaga livre e
-- Release/TMsrv/run/npc/Fadas revertido pelo git, o dbServer ressemeia a
-- prateleira no boot seguinte. O NPC em si segue desativado pela 0083.
UPDATE npc_config_meta SET version = version + 1 WHERE id = TRUE;
