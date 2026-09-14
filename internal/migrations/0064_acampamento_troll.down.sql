DELETE FROM drop_rule WHERE mob IN (
    'ATroll_Enigma', 'ATroll_Caos', 'ATroll_Mago', 'ATroll_Insano', 'ATroll_Cacador'
);
UPDATE drop_rule_meta SET version = version + 1 WHERE id = TRUE;

DELETE FROM npc_generator_off WHERE generator_index = 3804 AND turned_off_by = 'migração 0064';
UPDATE npc_generator_off_meta SET version = version + 1 WHERE id = TRUE;
