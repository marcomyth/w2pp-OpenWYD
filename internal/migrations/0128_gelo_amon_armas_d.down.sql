-- Os dois Amon voltam a não soltar Arma D. Apaga só os pares desta migração.
DELETE FROM drop_rule WHERE (mob, item) IN (VALUES
    ('Soldado_Amon', 810), ('Soldado_Amon', 825), ('Soldado_Amon', 840), ('Soldado_Amon', 855), ('Soldado_Amon', 870), ('Soldado_Amon', 885), ('Soldado_Amon', 900), ('Soldado_Amon', 911), ('Soldado_Amon', 936),
    ('Guerreiro_Amon', 810), ('Guerreiro_Amon', 825), ('Guerreiro_Amon', 840), ('Guerreiro_Amon', 855), ('Guerreiro_Amon', 870), ('Guerreiro_Amon', 885), ('Guerreiro_Amon', 900), ('Guerreiro_Amon', 911), ('Guerreiro_Amon', 936)
);

UPDATE drop_rule_meta SET version = version + 1 WHERE id = TRUE;
