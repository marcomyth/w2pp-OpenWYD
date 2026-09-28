-- Devolve o que a 0139 mudou: o Orc_L_Trooper volta à regra de 0,99% da 0136, os
-- outros dois Lordes Orc ao template, o Fragmento do FrenzyDemonLord aos 10% da
-- 0090, e o bloco 3135 volta a nascer. O MinuteGenerate do bloco 3134 volta pelo
-- NPCGener.txt revertido no git.
UPDATE drop_rule SET chance = 81, updated_at = now()
WHERE mob = 'Orc_L_Trooper' AND item = 1752;

DELETE FROM drop_rule WHERE (mob, item) IN (
    ('Lord_Trooper@@', 1752),
    ('OrcLordTrooper', 1752));

UPDATE drop_rule SET chance = 1000, updated_at = now()
WHERE mob = 'FrenzyDemonLord' AND item = 3224;

UPDATE drop_rule_meta SET version = version + 1 WHERE id = TRUE;

DELETE FROM npc_generator_off
WHERE generator_index = 3135 AND turned_off_by = 'migração 0139';

UPDATE npc_generator_off_meta SET version = version + 1 WHERE id = TRUE;
