-- 0108_deserto_saque — a mesa de saque do Deserto e os dois Agmo de evento.
--
-- O Deserto são as cinco caixas Deserto_* do Regions.txt: 433 monstros comuns de
-- onze templates, do nível 300 ao 380, e o Cav. Lugefer. Antes disto um comum
-- soltava algo além de peça de set a cada ~1.300 abates: as peças LE e a
-- Adamantita já tinham saído na 0080, e o resto do template são armas e sets a
-- 0,05%. Esta mesa dá à área um piso, e um degrau por profundidade. O template
-- continua valendo para tudo que não está aqui (as peças de set e as armas).
--
-- As chances estão em centésimos de por cento, como a Mesa grava e o painel
-- mostra. O sorteio da Mesa é rand() % 10000 sobre o rand() de 15 bits do MSVC, e
-- abaixo de 27,68% ele paga 4/3,2768 do escrito (internal/droprule/vies_test.go):
-- 100 aqui sai 1,22%, 33 sai 0,40%. Onde o número pago tem um limite pedido, ele
-- foi escrito já descontando isso.
--
-- Todo comum (onze templates): Poeira de Oriharucon 1%, de Lactolerium 0,5% e a
-- Moeda de Prata (1Mi) a 0,3%, a única entrada de ouro da área (monstro de
-- template não dá ouro, porque o Coin não é lido).
--
-- Por profundidade:
--   entrada (Tauron, Ladrão, Aranha)          Âmago Cavalo Leve N 0,33%, Classe C 0,5%
--   meio (Assassino, Arqueiro, Verme, Mantícora, Aeon, Treant)
--                                             Âmago Cavalo Leve N 0,33%, Classe D 0,5%
--   fundo (Lugefer, Adamant)                  Classe D 0,5%, Classe E 0,1%
-- O Cavalo Equipado N cai só dos Lugefer, pedido da equipe: âmago a 0,33% e ovo a
-- 0,05% na tropa (Lugefer), âmago a 3% e ovo a 0,4% no Cav. Lugefer. Ele não caía
-- de lugar nenhum desde que a 0090 o tirou do Submundo. Só a versão N: branca é
-- do gelo.
--
-- Fenrir no Ladrão e no Assassino, que já eram a fonte dele: âmago 0,2%, ovo
-- 0,05%. Uma gema por monstro a 0,15%: Garnet no Treant, Coral no Adamant,
-- Diamante no Aeon e no Verme, Esmeralda na Mantícora. A Pedra de Mantícora
-- (Pedra de Arch) desce de 0,2% para 0,04%, 1 a cada 2.048 abates.
--
-- O Cav. Lugefer: a Pedra do Lugefer estava na vaga 8, que o código fixa em 1 a
-- cada 4 mortes seja qual for o nível. A equipe pediu 1 pedra a cada 100 mortes
-- ou mais, nunca menos: 81 aqui paga 0,989% (1 a cada 101); 82 já pagaria
-- 1,001%. Andaluz N 2,5% pagos (Andaluz só de chefe, a 2-3%), Andaluz B fora,
-- Fragmento de Alma e Classe E de chefe, e Poeiras e Moeda em quantidade de chefe.
-- Os blocos dele passam de 10 para 20 monstros no NPCGener.txt, no mesmo commit,
-- pedido da equipe para deixar a área mais dura: MaxNumMob 2 nos dez blocos, e o
-- 3235 sai de MinuteGenerate -1 para 4, porque o boot levanta um grupo por bloco
-- e só o relógio do gerador chega ao segundo. MinuteGenerate conta passadas de
-- 12 s do relógio do legado (internal/spawnrate): 4 é 48 s, não 4 minutos.
--
-- Tauron comum tem 834 vagas também na Monster City, que é mapa de evento e não
-- gera bloco (internal/mapaevento): as regras dele só vazam num evento montado
-- lá. Os outros doze templates nascem só no Deserto.
INSERT INTO drop_rule (mob, item, chance) VALUES
    -- Todo comum: Poeiras e Moeda de Prata (1Mi).
    ('Tauron',          412,  100), ('Tauron',          413,   50), ('Tauron',          4026,  30),
    ('Ladrao_Tauron',   412,  100), ('Ladrao_Tauron',   413,   50), ('Ladrao_Tauron',   4026,  30),
    ('Aranha_Inferno',  412,  100), ('Aranha_Inferno',  413,   50), ('Aranha_Inferno',  4026,  30),
    ('Taron_Assassino', 412,  100), ('Taron_Assassino', 413,   50), ('Taron_Assassino', 4026,  30),
    ('Arqueiro_Tauron', 412,  100), ('Arqueiro_Tauron', 413,   50), ('Arqueiro_Tauron', 4026,  30),
    ('Verme_',          412,  100), ('Verme_',          413,   50), ('Verme_',          4026,  30),
    ('Manticora',       412,  100), ('Manticora',       413,   50), ('Manticora',       4026,  30),
    ('Aeon_Tauron',     412,  100), ('Aeon_Tauron',     413,   50), ('Aeon_Tauron',     4026,  30),
    ('Treant',          412,  100), ('Treant',          413,   50), ('Treant',          4026,  30),
    ('Lugefer',         412,  100), ('Lugefer',         413,   50), ('Lugefer',         4026,  30),
    ('Adamant_Tauron',  412,  100), ('Adamant_Tauron',  413,   50), ('Adamant_Tauron',  4026,  30),

    -- Entrada: Âmago de Cavalo Leve N e Classe C.
    ('Tauron',          2398,  33), ('Tauron',          4018,  50),
    ('Ladrao_Tauron',   2398,  33), ('Ladrao_Tauron',   4018,  50),
    ('Aranha_Inferno',  2398,  33), ('Aranha_Inferno',  4018,  50),

    -- Meio: Âmago de Cavalo Leve N e Classe D.
    ('Taron_Assassino', 2398,  33), ('Taron_Assassino', 4019,  50),
    ('Arqueiro_Tauron', 2398,  33), ('Arqueiro_Tauron', 4019,  50),
    ('Verme_',          2398,  33), ('Verme_',          4019,  50),
    ('Manticora',       2398,  33), ('Manticora',       4019,  50),
    ('Aeon_Tauron',     2398,  33), ('Aeon_Tauron',     4019,  50),
    ('Treant',          2398,  33), ('Treant',          4019,  50),

    -- Fundo: Classe D e Classe E.
    ('Lugefer',         4019,  50), ('Lugefer',         4020,  10),
    ('Adamant_Tauron',  4019,  50), ('Adamant_Tauron',  4020,  10),

    -- Cavalo Equipado N, só dos Lugefer.
    ('Lugefer',         2399,  33),       -- Âmago de Cavalo Equipado N
    ('Lugefer',         2309,   5),       -- Ovo de Cavalo Equipado N

    -- Fenrir.
    ('Ladrao_Tauron',   2406,  20), ('Ladrao_Tauron',   2316,   5),
    ('Taron_Assassino', 2406,  20), ('Taron_Assassino', 2316,   5),

    -- Uma gema por monstro.
    ('Treant',          2444,  15),       -- Garnet
    ('Adamant_Tauron',  2443,  15),       -- Coral
    ('Aeon_Tauron',     2441,  15),       -- Diamante
    ('Verme_',          2441,  15),       -- Diamante
    ('Manticora',       2442,  15),       -- Esmeralda

    -- Pedra de Arch na tropa: 1 a cada 2.048 abates.
    ('Manticora',       1756,   4),       -- Pedra de Mantícora

    -- O Cav. Lugefer.
    ('Cav._Lugefer',    1758,   81),      -- Pedra do Lugefer: 1 a cada 101 mortes
    ('Cav._Lugefer',    2400,  205),      -- Âmago de Andaluz N: 2,5% pagos
    ('Cav._Lugefer',    2405,    0),      -- Âmago de Andaluz B: branca é do gelo
    ('Cav._Lugefer',    2399,  300),      -- Âmago de Cavalo Equipado N
    ('Cav._Lugefer',    2309,   40),      -- Ovo de Cavalo Equipado N
    ('Cav._Lugefer',    3224,  500),      -- Fragmento de Alma
    ('Cav._Lugefer',    4020,  300),      -- Classe E
    ('Cav._Lugefer',    4026, 1000),      -- Moeda de Prata (1Mi)
    ('Cav._Lugefer',    412,  2500),      -- Poeira de Oriharucon
    ('Cav._Lugefer',    413,  1500),      -- Poeira de Lactolerium

    -- Os dois Agmo de evento soltam um pacote de âmagos N a cada morte:
    -- 40 Sem Sela, 30 Fantasma, 20 Cavalo Leve e 10 Cavalo Equipado. A Mesa diz
    -- que cai sempre; o tamanho do pacote mora em handler/deserto.go.
    ('Tauron_Agmo',     2396, 10000), ('Tauron_Agmo',   2397, 10000),
    ('Tauron_Agmo',     2398, 10000), ('Tauron_Agmo',   2399, 10000),
    ('Verme_Agmo',      2396, 10000), ('Verme_Agmo',    2397, 10000),
    ('Verme_Agmo',      2398, 10000), ('Verme_Agmo',    2399, 10000)
ON CONFLICT (mob, item) DO UPDATE SET chance = EXCLUDED.chance, updated_at = now();

UPDATE drop_rule_meta SET version = version + 1 WHERE id = TRUE;

-- Os Agmo passam a ser só de evento. Os dois blocos (3451 Verme_Agmo, 3452
-- Tauron_Agmo) têm MinuteGenerate -1, que no legado nunca nasce sozinho; o boot do
-- port os levantava e a fila de 15 s os devolvia, o que com um pacote de 100
-- âmagos por morte viraria uma torneira. Desligados pela chave da 0048, eles
-- continuam no mapa só quando a equipe os cria num evento ("/gm criar
-- Tauron_Agmo", que não renasce). TestBlocosDesligadosPorIndice prende os dois
-- índices aos dois templates.
INSERT INTO npc_generator_off (generator_index, turned_off_by) VALUES
    (3451, 'migração 0108'),
    (3452, 'migração 0108')
ON CONFLICT (generator_index) DO NOTHING;

UPDATE npc_generator_off_meta SET version = version + 1 WHERE id = TRUE;
