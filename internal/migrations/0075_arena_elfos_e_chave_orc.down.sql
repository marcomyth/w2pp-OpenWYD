DELETE FROM drop_rule
WHERE (mob IN ('Mestre_Elfo', 'Servo_Elfo') AND item IN (419, 420, 2392, 2393, 2394, 2395, 465))
   OR (mob IN ('Hidra_Dourada', 'Hidra_Imortal') AND item = 465);

UPDATE drop_rule_meta SET version = version + 1 WHERE id = TRUE;
