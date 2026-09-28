-- 0129_terra_mistica_sem_npc (down) — o Cap.Mercenario volta. Só apaga a linha
-- que a própria migração gravou: se um GM desligou o bloco à mão, fica.
DELETE FROM npc_generator_off WHERE generator_index = 985 AND turned_off_by = 'migração 0129';
UPDATE npc_generator_off_meta SET version = version + 1 WHERE id = TRUE;
