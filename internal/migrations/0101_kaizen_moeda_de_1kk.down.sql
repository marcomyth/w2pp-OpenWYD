-- Sem as regras, o laço de drop volta a ler os slots 40-44 do template do Cav.
-- Servo, que são cinco Moedas de Prata (5Mi) a 0,05% cada.
DELETE FROM drop_rule WHERE mob = 'Cav._Servo' AND item IN (4026, 4027);

UPDATE drop_rule_meta SET version = version + 1 WHERE id = TRUE;
