-- Devolve o que a 0156 mudou na Mesa: o Guer. Caveira e a escolta da Boss Hidra
-- Dourada voltam a soltar só o que o template diz, mais o Âmago de Urso da 0091 e
-- a Pedra do Esqueleto a 0% da 0140/0149, que esta migração não toca.
DELETE FROM drop_rule
WHERE mob IN ('Guer_Caveira', 'Guer_Caveira_Escolta') AND item IN (2392, 2393, 2395, 419);

UPDATE drop_rule_meta SET version = version + 1 WHERE id = TRUE;
