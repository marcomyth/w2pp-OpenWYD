DELETE FROM drop_rule
WHERE mob IN ('Cav._Kaizen', 'Cav._Servo', 'Hidra_Dourada', 'Hidra_Imortal')
  AND item IN (419, 420, 2392, 2393, 2394, 2395);

UPDATE drop_rule_meta SET version = version + 1 WHERE id = TRUE;
