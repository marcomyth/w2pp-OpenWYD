-- 0090_submundo_drops — a mesa de saque do Submundo: âmagos de montaria,
-- Repletions, Pergaminho da Água e o chefe.
--
-- As chances estão em centésimos de por cento, como a Mesa de Drops do painel
-- grava e mostra. Uma regra aqui SUBSTITUI o que o template faria para aquele
-- par (monstro, item) — é assim que as linhas a 0 tiram o Cavalo Equipado do
-- Submundo sem tocar no arquivo do monstro.
--
-- Âmagos, o que a área passa a pagar:
--   Sem Sela e Fantasma, os principais: N a 0,33% (1 em 303) e B a 0,29%
--   (1 em 345), nos NOVE monstros da área.
--   Cavalo Leve nas mesmas taxas, mas só nos dois Elfos Negros, que já eram a
--   fonte dele — é o que mantém Sem Sela e Fantasma como os principais.
--   Cavalo Equipado sai: âmago E ovo a 0% no Morlock e no Demon Gorgon, os dois
--   que o soltavam. O ovo entra junto porque entrega a montaria pronta, e
--   deixá-lo faria a regra do âmago não significar nada.
--
-- Pergaminho da Água (N) LV1 a 0,03% no Aqua Golem e no Morlock: 1 a cada 3.333
-- abates. A Mesa grava dois decimais, então 1 em 3.000 exato (0,0333%) não cabe
-- — 0,03% é o vizinho mais próximo.
--
-- Repletions a 0,5% (1 em 200): Classe C nos médios (135 a 275) e Classe D nos
-- fortes (310 a 399).
--
-- O chefe: o FrenzyDemonLord solta o Fragmento de Alma a 10% e a Pedra do Rei
-- Demonlord a 3% (vinha de 0,112%, que é a vaga 8 do template dele). O respawn
-- dele vai de 40 minutos a 4 horas no NPCGener.txt, no mesmo commit — o segundo
-- bloco dele (3135) é de evento (MinuteGenerate -1) e continua sem renascer.
--
-- Dois monstros que nascem dentro das coordenadas do Submundo ficam de fora,
-- pelas mesmas razões da 0089: Mestre Elfo e Servo Elfo são a arena dos Elfos da
-- Quest 256, com saque próprio desde a 0075, e a Flame Gargula tem sete blocos
-- fora da área — uma regra da Mesa vale por template_name, e o saque vazaria
-- para lá.
INSERT INTO drop_rule (mob, item, chance) VALUES
    ('Argos_Errante', 2396,   33),       -- Âmago de Cav s/Sela N
    ('Argos_Errante', 2397,   33),       -- Âmago de Cav Fantasm N
    ('Argos_Errante', 2401,   29),       -- Âmago de Ca s/Sela B
    ('Argos_Errante', 2402,   29),       -- Âmago de Cav Fantasm B
    ('Troll_Ghoul', 2396,   33),         -- Âmago de Cav s/Sela N
    ('Troll_Ghoul', 2397,   33),         -- Âmago de Cav Fantasm N
    ('Troll_Ghoul', 2401,   29),         -- Âmago de Ca s/Sela B
    ('Troll_Ghoul', 2402,   29),         -- Âmago de Cav Fantasm B
    ('Aqua_Golem', 2396,   33),          -- Âmago de Cav s/Sela N
    ('Aqua_Golem', 2397,   33),          -- Âmago de Cav Fantasm N
    ('Aqua_Golem', 2401,   29),          -- Âmago de Ca s/Sela B
    ('Aqua_Golem', 2402,   29),          -- Âmago de Cav Fantasm B
    ('Morlock', 2396,   33),             -- Âmago de Cav s/Sela N
    ('Morlock', 2397,   33),             -- Âmago de Cav Fantasm N
    ('Morlock', 2401,   29),             -- Âmago de Ca s/Sela B
    ('Morlock', 2402,   29),             -- Âmago de Cav Fantasm B
    ('Demon_Gorgon', 2396,   33),        -- Âmago de Cav s/Sela N
    ('Demon_Gorgon', 2397,   33),        -- Âmago de Cav Fantasm N
    ('Demon_Gorgon', 2401,   29),        -- Âmago de Ca s/Sela B
    ('Demon_Gorgon', 2402,   29),        -- Âmago de Cav Fantasm B
    ('CH_Troll_Ghoul', 2396,   33),      -- Âmago de Cav s/Sela N
    ('CH_Troll_Ghoul', 2397,   33),      -- Âmago de Cav Fantasm N
    ('CH_Troll_Ghoul', 2401,   29),      -- Âmago de Ca s/Sela B
    ('CH_Troll_Ghoul', 2402,   29),      -- Âmago de Cav Fantasm B
    ('Cav._Elfo_Negro', 2396,   33),     -- Âmago de Cav s/Sela N
    ('Cav._Elfo_Negro', 2397,   33),     -- Âmago de Cav Fantasm N
    ('Cav._Elfo_Negro', 2401,   29),     -- Âmago de Ca s/Sela B
    ('Cav._Elfo_Negro', 2402,   29),     -- Âmago de Cav Fantasm B
    ('Elfo_Negro_Abj', 2396,   33),      -- Âmago de Cav s/Sela N
    ('Elfo_Negro_Abj', 2397,   33),      -- Âmago de Cav Fantasm N
    ('Elfo_Negro_Abj', 2401,   29),      -- Âmago de Ca s/Sela B
    ('Elfo_Negro_Abj', 2402,   29),      -- Âmago de Cav Fantasm B
    ('FrenzyDemonLord', 2396,   33),     -- Âmago de Cav s/Sela N
    ('FrenzyDemonLord', 2397,   33),     -- Âmago de Cav Fantasm N
    ('FrenzyDemonLord', 2401,   29),     -- Âmago de Ca s/Sela B
    ('FrenzyDemonLord', 2402,   29),     -- Âmago de Cav Fantasm B
    ('Cav._Elfo_Negro', 2398,   33),     -- Âmago de Cavalo Leve N
    ('Cav._Elfo_Negro', 2403,   29),     -- Âmago de Cavalo Leve B
    ('Elfo_Negro_Abj', 2398,   33),      -- Âmago de Cavalo Leve N
    ('Elfo_Negro_Abj', 2403,   29),      -- Âmago de Cavalo Leve B
    ('Morlock', 2399,    0),             -- Âmago de Cavalo Equip N (fora)
    ('Morlock', 2404,    0),             -- Âmago de Cavalo Equip B (fora)
    ('Morlock', 2309,    0),             -- Ovo de Cavalo Equip N (fora)
    ('Morlock', 2314,    0),             -- Ovo de Cavalo Equip B (fora)
    ('Demon_Gorgon', 2399,    0),        -- Âmago de Cavalo Equip N (fora)
    ('Demon_Gorgon', 2404,    0),        -- Âmago de Cavalo Equip B (fora)
    ('Demon_Gorgon', 2309,    0),        -- Ovo de Cavalo Equip N (fora)
    ('Demon_Gorgon', 2314,    0),        -- Ovo de Cavalo Equip B (fora)
    ('Aqua_Golem', 3173,    3),          -- Pergaminho da Água(N)LV1
    ('Morlock', 3173,    3),             -- Pergaminho da Água(N)LV1
    ('Argos_Errante', 4018,   50),       -- Classe C
    ('Troll_Ghoul', 4018,   50),         -- Classe C
    ('Aqua_Golem', 4018,   50),          -- Classe C
    ('Morlock', 4018,   50),             -- Classe C
    ('Demon_Gorgon', 4018,   50),        -- Classe C
    ('CH_Troll_Ghoul', 4018,   50),      -- Classe C
    ('Cav._Elfo_Negro', 4019,   50),     -- Classe D
    ('Elfo_Negro_Abj', 4019,   50),      -- Classe D
    ('FrenzyDemonLord', 4019,   50),     -- Classe D
    ('FrenzyDemonLord', 3224, 1000),     -- Fragmento de Alma
    ('FrenzyDemonLord', 1759,  300)      -- Pedra do Rei Demonlord
ON CONFLICT (mob, item) DO UPDATE SET chance = EXCLUDED.chance, updated_at = now();

UPDATE drop_rule_meta SET version = version + 1 WHERE id = TRUE;
