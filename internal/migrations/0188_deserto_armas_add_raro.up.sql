-- 0188_deserto_armas_add_raro — pedido do Marco em 30/09/2026.
--
-- "Em 1 hora dropei 1 Cruz 72 e 1 Divino 54. Isso é muito forte." E: "podemos
-- adicionar mais armas com add randômico junto [...] porém precisa ficar ao
-- menos 5 horas para cair algo legal, e não minutos."
--
-- O add muda no código (handler/armas_add_tropa.go): a escada da tropa vai de
-- 27 a 72 de dano (12 a 32 de magia), e 54 ou mais sai em 4,3% das armas, contra
-- 60% antes. Aqui só a chance: as Armas D dos cinco monstros do Deserto ficam em
-- 1,5% de arma por morte, cada monstro com a chance repartida por igual entre
-- as armas que solta.
--
--   Mantícora e Adamant: seis armas cada, 21 por arma  -> 1,54% por morte
--     (era a mesma soma, 33 nas da 0180 e 9 nas da 0186)
--   Verme e Aeon: três armas cada, 41 por arma        -> 1,50% por morte
--     (Verme era 0,84%; Aeon, 0,33%)
--   Taron Assassino: dez armas, 12 por arma            -> 1,46% por morte
--     (era 0,98%)
--
-- A Mesa sorteia rand() % 10000 sobre o rand() de 15 bits do MSVC e, abaixo de
-- 27,68%, paga 4/3,2768 do escrito (internal/droprule/vies_test.go): 21 paga
-- 0,256%, 41 paga 0,500% e 12 paga 0,146%.
--
-- Com 300 mortes por hora (o ritmo dos logs de quem farma o Verme), são 4,5
-- armas por hora, uma de 54+ a cada 5 h, 63+ a cada 14 h e 72 a cada 55 h.
INSERT INTO drop_rule (mob, item, chance) VALUES
    ('Manticora',       870, 21),  -- Espada Vorpal
    ('Manticora',       911, 21),  -- Solaris
    ('Manticora',       810, 21),  -- Martelo Assassino
    ('Manticora',       869, 21),  -- Gram
    ('Manticora',       910, 21),  -- Luna
    ('Manticora',       809, 21),  -- Martelo Dragão

    ('Adamant_Tauron',  936, 21),  -- Mjolnir
    ('Adamant_Tauron',  855, 21),  -- Lança do Triunfo
    ('Adamant_Tauron',  902, 21),  -- Cajado de Âmbar
    ('Adamant_Tauron',  935, 21),  -- Martelo Psíquico
    ('Adamant_Tauron',  854, 21),  -- Gungnir
    ('Adamant_Tauron',  900, 21),  -- Fúria Divina

    ('Verme_',          885, 41),  -- Cruz Sagrada
    ('Verme_',          884, 41),  -- Lança Relâmpago
    ('Verme_',          825, 41),  -- Arco Divino

    ('Aeon_Tauron',     900, 41),  -- Fúria Divina
    ('Aeon_Tauron',     825, 41),  -- Arco Divino
    ('Aeon_Tauron',     840, 41),  -- Garra Draconiana

    ('Taron_Assassino', 869, 12),  -- Gram
    ('Taron_Assassino', 910, 12),  -- Luna
    ('Taron_Assassino', 809, 12),  -- Martelo Dragão
    ('Taron_Assassino', 935, 12),  -- Martelo Psíquico
    ('Taron_Assassino', 854, 12),  -- Gungnir
    ('Taron_Assassino', 902, 12),  -- Cajado de Âmbar
    ('Taron_Assassino', 899, 12),  -- Olho do Carbunkle
    ('Taron_Assassino', 824, 12),  -- Arco Élfico
    ('Taron_Assassino', 839, 12),  -- Presas de Behemoth
    ('Taron_Assassino', 884, 12)   -- Lança Relâmpago
ON CONFLICT (mob, item) DO UPDATE SET chance = EXCLUDED.chance, updated_at = now();

UPDATE drop_rule_meta SET version = version + 1 WHERE id = TRUE;
