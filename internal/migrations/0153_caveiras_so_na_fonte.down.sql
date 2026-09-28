-- Desfaz a 0153: as cópias da sala da fonte saem da Mesa, e a Caveira Lanc e o
-- Conj Caveira voltam a soltar os Restos e as Armas C da 0144, nas chances dela.
DELETE FROM drop_rule WHERE mob IN ('Caveira_Lanc_Fonte', 'Conj_Caveira_Fonte');

INSERT INTO drop_rule (mob, item, chance)
SELECT m.mob, i.item, CASE i.item WHEN 419 THEN 100 WHEN 420 THEN 50 ELSE 5 END
FROM (VALUES ('Caveira_Lanc'), ('Conj_Caveira')) AS m(mob),
     unnest(ARRAY[419, 420, 807, 808, 822, 823, 837, 838, 867, 868, 882, 883, 908, 909, 933, 934, 852, 853, 897, 898, 901]::smallint[]) AS i(item)
ON CONFLICT (mob, item) DO UPDATE SET chance = EXCLUDED.chance, updated_at = now();

UPDATE drop_rule_meta SET version = version + 1 WHERE id = TRUE;
