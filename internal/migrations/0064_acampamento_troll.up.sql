-- 0064_acampamento_troll — o saque da quest do Acampamento Troll, e o Troll
-- Enigma do mundo fora do mapa.
--
-- Pedido da equipe em 14/09/2026: uma quest focada em Armas D, âmagos e ovos de
-- Cavalo Fantasma, com os Trolls do acampamento e o Enigma nos números do
-- Castelo Orc. Os cinco templates ATroll_* (Release/TMsrv/run/npc) nascem sem
-- drop próprio: tudo o que cai vem daqui, na chance exata da Mesa de Drops, e
-- continua editável em /drops. O add das armas — a Mesa não diz valor — é do
-- tmServer (handler/acampamento_troll.go).
--
-- A conta de uma entrada supõe 40 de tropa, 2 guardiões, o boss e ~44 seguidores
-- (4 no começo e mais 4 a cada 30 s, em uns 5 min perto do boss). Chance em
-- centésimos de por cento:
--
--   Meta por entrada                  tropa   seguidor  guardião  boss
--   ~6   Armas D (8 tipos)            0,5%    0,5%      10%       12,5%  cada tipo
--   ~4,2 Repletion D (Classe D 4019)  10%     —         10%       —
--   ~3,5 Poeira de Oriharucon 412     7,5%    —         7,5%      30%
--   ~1,4 Poeira de Lactolerium 413    2,8%    —         2,8%      25%
--   ~5,2 Âmago Cav. s/ Sela N 2396    3%      9%        —         —
--   ~3,9 Âmago Cav. s/ Sela B 2401    2%      7%        —         —
--   ~2,7 Âmago Cav. Fantasma N 2397   —       5%        25%       —
--   ~1,6 Âmago Cav. Fantasma B 2402   —       3%        15%       —
--   ~0,4 Ovo Cav. Fantasma N+B        —       —         —         25% + 12,5%
--
-- Os ovos de Cavalo Fantasma (2307 N, 2312 B) caem só do boss: 1 a cada ~2,7
-- entradas.
--
-- As armas são as D de nível mais baixo, a primeira de cada par D do catálogo:
-- Gram 869, Martelo Dragão 809, Luna 910, Arco Élfico 824, Martelo Psíquico 935
-- (físicas) e Olho do Carbunkle 899, Gungnir 854, Cajado de Âmbar 902 (mágicas).
--
-- ON CONFLICT DO NOTHING, como na 0053: uma linha que alguém já gravou pelo
-- painel antes do deploy vale mais que a proposta.

INSERT INTO drop_rule (mob, item, chance) VALUES
    -- Tropa: Troll Insano e Caçador Troll.
    ('ATroll_Insano',   869,   50), ('ATroll_Cacador',  869,   50),
    ('ATroll_Insano',   809,   50), ('ATroll_Cacador',  809,   50),
    ('ATroll_Insano',   910,   50), ('ATroll_Cacador',  910,   50),
    ('ATroll_Insano',   824,   50), ('ATroll_Cacador',  824,   50),
    ('ATroll_Insano',   935,   50), ('ATroll_Cacador',  935,   50),
    ('ATroll_Insano',   899,   50), ('ATroll_Cacador',  899,   50),
    ('ATroll_Insano',   854,   50), ('ATroll_Cacador',  854,   50),
    ('ATroll_Insano',   902,   50), ('ATroll_Cacador',  902,   50),
    ('ATroll_Insano',  4019, 1000), ('ATroll_Cacador', 4019, 1000),
    ('ATroll_Insano',   412,  750), ('ATroll_Cacador',  412,  750),
    ('ATroll_Insano',   413,  280), ('ATroll_Cacador',  413,  280),
    ('ATroll_Insano',  2396,  300), ('ATroll_Cacador', 2396,  300),
    ('ATroll_Insano',  2401,  200), ('ATroll_Cacador', 2401,  200),
    -- Seguidores: Troll Mago, que renasce perto do boss.
    ('ATroll_Mago',   869,   50), ('ATroll_Mago',   809,   50),
    ('ATroll_Mago',   910,   50), ('ATroll_Mago',   824,   50),
    ('ATroll_Mago',   935,   50), ('ATroll_Mago',   899,   50),
    ('ATroll_Mago',   854,   50), ('ATroll_Mago',   902,   50),
    ('ATroll_Mago',  2396,  900), ('ATroll_Mago',  2401,  700),
    ('ATroll_Mago',  2397,  500), ('ATroll_Mago',  2402,  300),
    -- Guardiões: Troll Caos.
    ('ATroll_Caos',   869, 1000), ('ATroll_Caos',   809, 1000),
    ('ATroll_Caos',   910, 1000), ('ATroll_Caos',   824, 1000),
    ('ATroll_Caos',   935, 1000), ('ATroll_Caos',   899, 1000),
    ('ATroll_Caos',   854, 1000), ('ATroll_Caos',   902, 1000),
    ('ATroll_Caos',  4019, 1000),
    ('ATroll_Caos',   412,  750), ('ATroll_Caos',   413,  280),
    ('ATroll_Caos',  2397, 2500), ('ATroll_Caos',  2402, 1500),
    -- Boss: Troll Enigma. Os ovos de Cavalo Fantasma só caem dele.
    ('ATroll_Enigma',  869, 1250), ('ATroll_Enigma',  809, 1250),
    ('ATroll_Enigma',  910, 1250), ('ATroll_Enigma',  824, 1250),
    ('ATroll_Enigma',  935, 1250), ('ATroll_Enigma',  899, 1250),
    ('ATroll_Enigma',  854, 1250), ('ATroll_Enigma',  902, 1250),
    ('ATroll_Enigma', 2307, 2500), ('ATroll_Enigma', 2312, 1250),
    ('ATroll_Enigma',  412, 3000), ('ATroll_Enigma',  413, 2500)
ON CONFLICT (mob, item) DO NOTHING;

UPDATE drop_rule_meta SET version = version + 1 WHERE id = TRUE;

-- O Troll Enigma do mundo (bloco 3804, na jaula do acampamento) sai: a quest tem o
-- dela no mesmo lugar. O bloco NÃO sai do NPCGener, porque o índice de um bloco é
-- a posição dele no arquivo; ele desliga pela chave da 0048, a mesma do
-- "/gm npc off", e volta com "/gm npc on 3804". TestBlocosDesligadosPorIndice
-- prende o 3804 ao Troll_Enigma.
INSERT INTO npc_generator_off (generator_index, turned_off_by) VALUES (3804, 'migração 0064')
ON CONFLICT (generator_index) DO NOTHING;

UPDATE npc_generator_off_meta SET version = version + 1 WHERE id = TRUE;
