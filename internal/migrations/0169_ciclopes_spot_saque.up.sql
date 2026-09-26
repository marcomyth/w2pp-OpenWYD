-- 0169_ciclopes_spot_saque — o saque novo dos Ciclopes Cruéis e do spot deles.
--
-- Pedido do Marco em 26/09/2026, com print em (2239,1333). O spot é x 2195-2285,
-- y 1300-1380, e nele os blocos usam as cópias Ciclope_Cruel_Spot e
-- Lanceiro_Zakum_Spot (NPCGener.txt), iguais ao original no nome que o jogador vê;
-- a Mesa vale por template, e é pelas cópias que o saque do spot fica no spot.
--
-- As chances são as do conjunto "moderado", já compensadas pelo viés do rand() do
-- MSVC, que infla em 22,07% toda chance abaixo de 27,68% (droprule/vies_test.go,
-- a mesma conta da 0099 e da 0149): 82 dá 1,00%, 41 dá 0,50% e 4 dá 0,05%.
--
--   todo Ciclope Cruel do mapa (o original e a cópia do spot):
--     Moeda de Prata (1Mi) 4026          1%
--     Resto de Oriharucon 419            1%
--     Resto de Lactolerium 420           0,5%
--     Âmago de Dente de Sabre 2395       1%
--     Âmago de Cav s/Sela N 2396, B 2401 0,05% cada
--     Âmago de Cav Fantasma N 2397, B 2402 0,05% cada
--
--   o Lanceiro Zakum do spot:
--     Moeda de Prata (1Mi), Restos       as mesmas do Ciclope
--
--   as duas cópias do spot:
--     as 19 Armas C                      0,05% cada (0,93% no total); o add
--                                        (dano 45-72, magia 20-32) é do código
--                                        (handler/ciclopes.go, ciclopesFinish)
--
-- Uma regra para um item que já está na bolsa do template toma o lugar dele: as
-- três Armas C que o Ciclope Cruel já trazia passam a sair pela regra, no spot.
-- O chefe do spot, o Ciclope Tirano, não tem linha aqui: o saque dele é do código.
INSERT INTO drop_rule (mob, item, chance) VALUES
    ('Ciclope_Cruel', 4026, 82),
    ('Ciclope_Cruel',  419, 82),
    ('Ciclope_Cruel',  420, 41),
    ('Ciclope_Cruel', 2395, 82),
    ('Ciclope_Cruel', 2396,  4),
    ('Ciclope_Cruel', 2401,  4),
    ('Ciclope_Cruel', 2397,  4),
    ('Ciclope_Cruel', 2402,  4),

    ('Ciclope_Cruel_Spot', 4026, 82),
    ('Ciclope_Cruel_Spot',  419, 82),
    ('Ciclope_Cruel_Spot',  420, 41),
    ('Ciclope_Cruel_Spot', 2395, 82),
    ('Ciclope_Cruel_Spot', 2396,  4),
    ('Ciclope_Cruel_Spot', 2401,  4),
    ('Ciclope_Cruel_Spot', 2397,  4),
    ('Ciclope_Cruel_Spot', 2402,  4),

    ('Lanceiro_Zakum_Spot', 4026, 82),
    ('Lanceiro_Zakum_Spot',  419, 82),
    ('Lanceiro_Zakum_Spot',  420, 41)
ON CONFLICT (mob, item) DO UPDATE SET chance = EXCLUDED.chance, updated_at = now();

-- As Armas C das duas cópias do spot: 14 físicas e 5 mágicas, 0,05% cada.
INSERT INTO drop_rule (mob, item, chance) VALUES
    ('Ciclope_Cruel_Spot', 807, 4), ('Ciclope_Cruel_Spot', 808, 4), ('Ciclope_Cruel_Spot', 822, 4), ('Ciclope_Cruel_Spot', 823, 4), ('Ciclope_Cruel_Spot', 837, 4), ('Ciclope_Cruel_Spot', 838, 4), ('Ciclope_Cruel_Spot', 867, 4),
    ('Ciclope_Cruel_Spot', 868, 4), ('Ciclope_Cruel_Spot', 882, 4), ('Ciclope_Cruel_Spot', 883, 4), ('Ciclope_Cruel_Spot', 908, 4), ('Ciclope_Cruel_Spot', 909, 4), ('Ciclope_Cruel_Spot', 933, 4), ('Ciclope_Cruel_Spot', 934, 4),
    ('Ciclope_Cruel_Spot', 852, 4), ('Ciclope_Cruel_Spot', 853, 4), ('Ciclope_Cruel_Spot', 897, 4), ('Ciclope_Cruel_Spot', 898, 4), ('Ciclope_Cruel_Spot', 901, 4),
    ('Lanceiro_Zakum_Spot', 807, 4), ('Lanceiro_Zakum_Spot', 808, 4), ('Lanceiro_Zakum_Spot', 822, 4), ('Lanceiro_Zakum_Spot', 823, 4), ('Lanceiro_Zakum_Spot', 837, 4), ('Lanceiro_Zakum_Spot', 838, 4), ('Lanceiro_Zakum_Spot', 867, 4),
    ('Lanceiro_Zakum_Spot', 868, 4), ('Lanceiro_Zakum_Spot', 882, 4), ('Lanceiro_Zakum_Spot', 883, 4), ('Lanceiro_Zakum_Spot', 908, 4), ('Lanceiro_Zakum_Spot', 909, 4), ('Lanceiro_Zakum_Spot', 933, 4), ('Lanceiro_Zakum_Spot', 934, 4),
    ('Lanceiro_Zakum_Spot', 852, 4), ('Lanceiro_Zakum_Spot', 853, 4), ('Lanceiro_Zakum_Spot', 897, 4), ('Lanceiro_Zakum_Spot', 898, 4), ('Lanceiro_Zakum_Spot', 901, 4)
ON CONFLICT (mob, item) DO UPDATE SET chance = EXCLUDED.chance, updated_at = now();

UPDATE drop_rule_meta SET version = version + 1 WHERE id = TRUE;
