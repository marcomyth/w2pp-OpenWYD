-- 0140_esqueleto_e_boss_dragao_lich — a Pedra do Esqueleto sai do jogo, a Pedra de
-- Dragão Lich passa a ser de um chefe novo, e o Dragão Lich comum fica 40% mais forte.
--
-- Pedido do Marco em 25/09/2026, depois da simulação da HT Xorimpas com as chances
-- da 0136 e da 0139.
--
-- A Pedra do Esqueleto (1753) vai a 0% nos três templates que a carregam. O
-- Guer_Caveira tem 104 bichos nos andares 1 e 2 da Dungeon, morre com um golpe e
-- volta em 12 a 24 s: com a 0136 a HT tirava uma pedra a cada ~19 minutos. O
-- Cav.Caveira (1 em 16 mil pelo template) e o SkeltonWarrior (sem bloco) entram
-- para ela não voltar por eles.
--
-- A Pedra de Dragão Lich (1754) sai do Dragão Lich e do BoneDragon (sem bloco) e
-- passa a ser do Boss Dragão Lich (template Boss_Dragao_Lich, bloco 6146, no salão
-- de cima dos Dragões no 3º andar), a 81 = 0,99% por morte, a régua das outras
-- Pedras Arch. O resto do saque dele e o renascimento de 4 horas são código
-- (handler/dungeon.go).
--
-- O Dragão Lich comum: +40% de vida e de dano, de 26.000 para 36.400 e de 1.100
-- para 1.540, no arquivo do template. Se o painel tiver gravado uma ficha para
-- ele, a ficha substitui o arquivo inteiro no boot (mobstat.Apply), então ela
-- recebe os mesmos números. Absolutos, e não "× 1,4" sobre a coluna: uma conta
-- sobre o valor antigo compõe se a migração rodar duas vezes
-- (upsert_absoluto_test.go).
INSERT INTO drop_rule (mob, item, chance) VALUES
    ('Guer_Caveira',     1753,  0),       -- Pedra do Esqueleto
    ('Cav.Caveira',      1753,  0),       -- Pedra do Esqueleto
    ('SkeltonWarrior',   1753,  0),       -- Pedra do Esqueleto
    ('Dragao_Lich',      1754,  0),       -- Pedra de Dragão Lich
    ('BoneDragon',       1754,  0),       -- Pedra de Dragão Lich
    ('Boss_Dragao_Lich', 1754, 81)        -- Pedra de Dragão Lich
ON CONFLICT (mob, item) DO UPDATE SET chance = EXCLUDED.chance, updated_at = now();

UPDATE drop_rule_meta SET version = version + 1 WHERE id = TRUE;

UPDATE mob_template_stat SET max_hp = 36400, hp = 36400, damage = 1540, updated_at = now()
WHERE template_name = 'Dragao_Lich';
