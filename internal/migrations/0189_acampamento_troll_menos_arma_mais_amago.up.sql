-- 0189_acampamento_troll_menos_arma_mais_amago — pedido do Marco em 30/09/2026.
--
-- Revisando os spots de arma com add, o Acampamento Troll dava ~7 Armas D por
-- entrada, ~5 delas da tropa e dos Magos (0,5% por tipo, 8 tipos: uma arma a
-- cada 20 mortes), e ~1,8 com 54+ de dano. Resposta dele: "podemos diminuir um
-- pouco e dar mais âmagos". O Troll Caos e o Enigma não mudam.
--
-- Tropa (Troll Insano, Caçador Troll) e seguidor (Troll Mago), em centésimos
-- de por cento:
--
--                                      tropa          Mago
--   cada uma das 8 Armas D             50 -> 25       50 -> 25
--   Âmago Cav. s/ Sela N 2396         300 -> 600     900 -> 1800
--   Âmago Cav. s/ Sela B 2401         200 -> 400     700 -> 1400
--   Âmago Cav. Fantasma N 2397          — -> 200     500 -> 1000
--   Âmago Cav. Fantasma B 2402          — -> 100     300 -> 600
--
-- A escada de add da tropa (handler/acampamento_troll.go) fica como está.
-- Tropa e Mago soltam âmago UM por vez; o pacote é só do Caos.
--
-- Por entrada, supondo os 100 abates meio a meio entre tropa e Magos, mais 4
-- Caos e o Enigma (a Mesa paga 4/3,2768 do escrito abaixo de 27,68%):
--   Armas D                ~7,1 -> ~4,7   (54+: ~1,8 -> ~1,4; o Enigma dá ~0,8)
--   Âmago s/ Sela N        ~22  -> ~29
--   Âmago s/ Sela B        ~15  -> ~21
--   Âmago Fantasma N       ~10  -> ~15
--   Âmago Fantasma B       ~7   -> ~9
INSERT INTO drop_rule (mob, item, chance) VALUES
    ('ATroll_Insano',   869,   25), ('ATroll_Cacador',  869,   25), ('ATroll_Mago',   869,   25),
    ('ATroll_Insano',   809,   25), ('ATroll_Cacador',  809,   25), ('ATroll_Mago',   809,   25),
    ('ATroll_Insano',   910,   25), ('ATroll_Cacador',  910,   25), ('ATroll_Mago',   910,   25),
    ('ATroll_Insano',   824,   25), ('ATroll_Cacador',  824,   25), ('ATroll_Mago',   824,   25),
    ('ATroll_Insano',   935,   25), ('ATroll_Cacador',  935,   25), ('ATroll_Mago',   935,   25),
    ('ATroll_Insano',   899,   25), ('ATroll_Cacador',  899,   25), ('ATroll_Mago',   899,   25),
    ('ATroll_Insano',   854,   25), ('ATroll_Cacador',  854,   25), ('ATroll_Mago',   854,   25),
    ('ATroll_Insano',   902,   25), ('ATroll_Cacador',  902,   25), ('ATroll_Mago',   902,   25),

    ('ATroll_Insano',  2396,  600), ('ATroll_Cacador', 2396,  600), ('ATroll_Mago',  2396, 1800),
    ('ATroll_Insano',  2401,  400), ('ATroll_Cacador', 2401,  400), ('ATroll_Mago',  2401, 1400),
    ('ATroll_Insano',  2397,  200), ('ATroll_Cacador', 2397,  200), ('ATroll_Mago',  2397, 1000),
    ('ATroll_Insano',  2402,  100), ('ATroll_Cacador', 2402,  100), ('ATroll_Mago',  2402,  600)
ON CONFLICT (mob, item) DO UPDATE SET chance = EXCLUDED.chance, updated_at = now();

UPDATE drop_rule_meta SET version = version + 1 WHERE id = TRUE;
