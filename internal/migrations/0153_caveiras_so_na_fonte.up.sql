-- 0153_caveiras_so_na_fonte — o spot de caveiras da 0144 vale só na sala da foto.
--
-- Correção pedida pelo Marco em 25/09/2026. O pedido era a sala da fonte, com
-- prints em (353,3756), e a 6e60220e mexeu na Caveira Lanc e no Conj Caveira do
-- andar inteiro: 74 blocos (salas a oeste, a sala da fonte, a caverna a leste e o
-- corredor ao norte) e, pela Mesa, o saque dos dois templates em todo lugar.
--
-- No NPCGener.txt do mesmo commit, a sala é x 345-361, y 3740-3772, entre as
-- paredes do HeightMap: só os seis blocos dela (1884 a 1889) ficam 5x, com as
-- cópias Caveira_Lanc_Fonte e Conj_Caveira_Fonte, iguais byte a byte ao original.
-- Os outros 68 blocos voltam ao que eram antes da 6e60220e.
--
-- Aqui, o saque vai junto: as cópias recebem TUDO o que a Mesa dá hoje aos
-- originais — o Âmago de Lobo da 0091, os Restos e as Armas C da 0144, com a
-- chance que estiver valendo, afinação de painel incluída —, e depois os
-- originais perdem o que a 0144 deu. O Âmago de Lobo da 0091 fica nos dois.
INSERT INTO drop_rule (mob, item, chance)
SELECT 'Caveira_Lanc_Fonte', item, chance FROM drop_rule WHERE lower(mob) = 'caveira_lanc'
ON CONFLICT (mob, item) DO UPDATE SET chance = EXCLUDED.chance, updated_at = now();

INSERT INTO drop_rule (mob, item, chance)
SELECT 'Conj_Caveira_Fonte', item, chance FROM drop_rule WHERE lower(mob) = 'conj_caveira'
ON CONFLICT (mob, item) DO UPDATE SET chance = EXCLUDED.chance, updated_at = now();

DELETE FROM drop_rule
WHERE lower(mob) IN ('caveira_lanc', 'conj_caveira')
  AND item IN (419, 420, 807, 808, 822, 823, 837, 838, 867, 868, 882, 883, 908, 909, 933, 934, 852, 853, 897, 898, 901);

UPDATE drop_rule_meta SET version = version + 1 WHERE id = TRUE;
