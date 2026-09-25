-- 0136_pedras_arch_nerf — as Pedras Arch de monstro no máximo a 1%, a régua do
-- Cav. Lugefer.
--
-- Pedido do Marco em 25/09/2026: "nesse início vamos deixar bem nerfado". O
-- levantamento de quem solta as pedras 1752-1759 achou cinco monstros acima do
-- 1% que a 0109 deu ao Cav. Lugefer (81 = 0,99% por morte, 1 a cada 101):
--
--   Pedra                   Monstro          Onde                     Antes
--   1752 Lorde Orc          Orc_L_Trooper    Azran (10)               2,9% (slot 57 do template)
--   1753 Esqueleto          Guer_Caveira     Dungeon 1 e 2 (104)      ~3%  (slots 2, 17 e 57)
--   1755 Demonlord          Demon_Lord       Dungeon 3 (8)            25%  (slot 8, fixo)
--   1756 Mantícora          Boss_Manticora   Deserto (chefe)          9,95% (sorteio em deserto.go)
--   1759 Rei Demonlord      FrenzyDemonLord  Submundo (2)             3,66% (a 300 da 0090)
--
-- Todos vão a 81, o número do Lugefer. 81 e não 100 porque a Mesa sorteia com
-- rand() % 10000 e o rand() do MSVC não passa de 32767: abaixo de 27,68% toda
-- chance sai 22,1% maior do que o número gravado, e 81 é o que dá 0,99%.
--
-- Quem já estava abaixo de 1% fica como está, para o nerf não virar aumento:
-- Cav.Caveira (1753, ~1 em 16 mil), Dragao_Lich (1754, 0,24%), Flame_Gargula
-- (1757, 0,11%) e o próprio Cav. Lugefer (1758).
--
-- O Boss Mantícora não solta a pedra pelo template: ele sorteia um prêmio por
-- morte, e a pedra era a fatia de 10% (bossManticoraSaque). Com uma regra para
-- ele, a Mesa vale por cima do sorteio: a pedra rola à parte, a 0,99%, e a fatia
-- dela no sorteio passa a não entregar nada. Os outros prêmios continuam onde
-- estavam.
--
-- ON CONFLICT DO UPDATE: o pedido é fixar estes números, por cima do que o
-- painel tiver gravado para estes pares.
INSERT INTO drop_rule (mob, item, chance) VALUES
    ('Orc_L_Trooper',   1752, 81),        -- Pedra do Lorde Orc
    ('Guer_Caveira',    1753, 81),        -- Pedra do Esqueleto
    ('Demon_Lord',      1755, 81),        -- Pedra do Demonlord
    ('Boss_Manticora',  1756, 81),        -- Pedra de Mantícora
    ('FrenzyDemonLord', 1759, 81)         -- Pedra do Rei Demonlord
ON CONFLICT (mob, item) DO UPDATE SET chance = EXCLUDED.chance, updated_at = now();

UPDATE drop_rule_meta SET version = version + 1 WHERE id = TRUE;
