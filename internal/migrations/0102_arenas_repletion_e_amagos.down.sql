-- Tira o Repletion das três arenas e devolve os Âmagos às chances da 0065/0075.
DELETE FROM drop_rule
 WHERE item IN (4018, 4019)
   AND mob IN ('Cav._Kaizen', 'Cav._Servo', 'Hidra_Dourada', 'Hidra_Imortal', 'Mestre_Elfo', 'Servo_Elfo');

UPDATE drop_rule SET chance = 600, updated_at = now()
 WHERE item IN (2392, 2393, 2394, 2395)
   AND mob IN ('Cav._Kaizen', 'Hidra_Dourada', 'Mestre_Elfo');
UPDATE drop_rule SET chance = 150, updated_at = now()
 WHERE item IN (2392, 2393, 2394, 2395)
   AND mob IN ('Cav._Servo', 'Hidra_Imortal', 'Servo_Elfo');

UPDATE drop_rule_meta SET version = version + 1 WHERE id = TRUE;
