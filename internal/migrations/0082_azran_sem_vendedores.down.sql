-- Devolve a fila de Azran ao mundo.
UPDATE npc_definition SET enabled = TRUE
WHERE template_name IN (
    'Ferreiro_Azran', 'Rainy_Azran', 'Rapein_Azran', 'Arnod_Azran',
    'ArmaMortalAzran', 'ArmaArchAzran', 'DonatesBars');

UPDATE npc_config_meta SET version = version + 1 WHERE id = TRUE;
