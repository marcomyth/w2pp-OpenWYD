-- Desfaz a 0144: nenhuma destas linhas existia antes (a 0091 só deu o Âmago de
-- Lobo aos dois), então elas saem inteiras.
DELETE FROM drop_rule
WHERE mob IN ('Caveira_Lanc', 'Conj_Caveira')
  AND item IN (419, 420, 807, 808, 822, 823, 837, 838, 867, 868, 882, 883, 908, 909, 933, 934, 852, 853, 897, 898, 901);

UPDATE drop_rule_meta SET version = version + 1 WHERE id = TRUE;
