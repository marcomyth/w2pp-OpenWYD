-- Devolve as Pedras Arch ao que eram antes da 0136: quatro monstros voltam ao
-- template (e o Boss Mantícora ao sorteio de deserto.go), e o FrenzyDemonLord
-- volta à regra de 3% da 0090.
DELETE FROM drop_rule WHERE (mob, item) IN (
    ('Orc_L_Trooper',  1752),
    ('Guer_Caveira',   1753),
    ('Demon_Lord',     1755),
    ('Boss_Manticora', 1756));

UPDATE drop_rule SET chance = 300, updated_at = now()
WHERE mob = 'FrenzyDemonLord' AND item = 1759;

UPDATE drop_rule_meta SET version = version + 1 WHERE id = TRUE;
