DELETE FROM drop_rule WHERE mob = 'ATroll_Caos' AND item IN (2396, 2401);
DELETE FROM drop_rule WHERE mob = 'ATroll_Enigma' AND item = 3173;
UPDATE drop_rule SET chance = 1000, updated_at = now() WHERE mob = 'ATroll_Caos' AND item IN (869, 809, 910, 824, 935, 899, 854, 902);
UPDATE drop_rule SET chance = 2500, updated_at = now() WHERE mob = 'ATroll_Caos' AND item = 2397;
UPDATE drop_rule SET chance = 1500, updated_at = now() WHERE mob = 'ATroll_Caos' AND item = 2402;
UPDATE drop_rule_meta SET version = version + 1 WHERE id = TRUE;
