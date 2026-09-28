-- Devolve o que a 0140 mudou na Mesa e na ficha do painel: o Guer_Caveira volta à
-- regra de 0,99% da 0136, os outros templates ao que o arquivo diz, e a ficha do
-- Dragão Lich, se houver, aos 26.000 de vida e 1.100 de dano. O arquivo do
-- template, o Boss_Dragao_Lich e o bloco 6146 voltam pelo git.
UPDATE drop_rule SET chance = 81, updated_at = now()
WHERE mob = 'Guer_Caveira' AND item = 1753;

DELETE FROM drop_rule WHERE (mob, item) IN (
    ('Cav.Caveira',      1753),
    ('SkeltonWarrior',   1753),
    ('Dragao_Lich',      1754),
    ('BoneDragon',       1754),
    ('Boss_Dragao_Lich', 1754));

UPDATE drop_rule_meta SET version = version + 1 WHERE id = TRUE;

UPDATE mob_template_stat SET max_hp = 26000, hp = 26000, damage = 1100, updated_at = now()
WHERE template_name = 'Dragao_Lich';
