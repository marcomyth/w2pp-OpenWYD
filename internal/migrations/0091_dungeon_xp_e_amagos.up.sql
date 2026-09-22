-- 0091_dungeon_xp_e_amagos — teto de 10.000 de XP na Dungeon inteira e os
-- âmagos de montaria distribuídos em escada por andar.
--
-- XP: trinta templates dos três andares passam a pagar no máximo 10.000. Vinham
-- de 15.000 a 2.990.849 — o Demon Lord pagava três milhões por abate, e o
-- Cavaleiro Negro 281.560 com 35 vivos renascendo a cada dois minutos. O Dragão
-- de Cobre (2.700) e o Dragão Selvagem (3.500) já estavam abaixo do teto e não
-- foram tocados. O Demon Lord passa a renascer de 40 em 40 minutos em vez de 20,
-- nos dois blocos dele (2650 e 2651), no NPCGener.txt do mesmo commit.
--
-- Âmagos, escada por andar, nas taxas do Submundo (0091 é irmã da 0090):
--   1º andar  Lobo, Urso e Dragão Menor a 0,50% (1 em 200), por faixa de nível
--             dentro do andar: Lobo nos de 84 a 120, Urso nos de 126 a 136 e
--             Dragão Menor nos de 160 a 255.
--   2º andar  Sem Sela N a 0,33% e B a 0,29%.
--   3º andar  Fantasma e Leve, N a 0,33% e B a 0,29%.
--
-- Gárgula, Golem de Fogo, Guer. Caveira, Anf. Assassino, Bruxa Élfica e Gárgula
-- Sábio nascem em mais de um andar, e uma regra da Mesa vale por template: eles
-- seguem o andar MAIS BAIXO em que aparecem, que é onde a escada os coloca.
--
-- Três monstros do 3º andar ficam de fora dos âmagos: Dragão de Cobre, Dragão
-- Selvagem e Dragão Amald têm Merchant 16, e mob com Merchant != 0 não recebe
-- dano no port (city.go, combat.go) — drop neles seria letra morta. A XP deles
-- entrou no teto assim mesmo, que é barato e fecha a regra da área.
--
-- Fora daqui, como na 0089 e na 0090: as arenas da Quest 256 que ficam dentro
-- das caixas da Dungeon — Cav. Kaizen e Cav. Servo no 1º andar, Hidra Dourada e
-- Hidra Imortal também no 1º. São conteúdo de quest paga, com saque próprio
-- desde a 0065. A Hidra Dourada_ (com o sublinhado) é outra coisa: nasce só na
-- Dungeon e entra normalmente.
--
-- Um vazamento conhecido e aceito: o Arq. Caveira tem 24 bichos na Dungeon e 5
-- no Pesadelo N, e a Exp mora no template — os cinco do Pesadelo passam a pagar
-- 10.000 também.
UPDATE mob_template_stat SET exp = 10000, updated_at = now()
WHERE exp > 10000 AND template_name IN (
    'Patrulha', 'Patrulha_', 'Patrulha__', 'Caveira', 'Urso_Zumbi', 'Troll_Zumbi', 'Arq_Caveira',
    'Caveira_Lanc', 'Conj_Caveira', 'Hidra', 'Cav.Caveira', 'Hidra_Dourada_', 'Elfo_Negro',
    'Guer_Caveira', 'Anf_Assassino', 'Bruxa_Elfica', 'Gargula_Sabio', 'Gargula', 'Golem_de_Fogo',
    'Anf_Ninja', 'Grim_Lock', 'Gargula_Inf', 'Gargula_Servo', 'Golem_de_Pedra',
    'Hezling', 'Dragao_Lich', 'Dragao_de_Cobre', 'Cav._Mortal', 'Dragao_Selvagem', 'Dragao_Amald',
    'Cavaleiro_Negro', 'Demon_Lord');

INSERT INTO drop_rule (mob, item, chance) VALUES
    ('Caveira', 2392,  50),            -- Âmago de Lobo
    ('Urso_Zumbi', 2392,  50),         -- Âmago de Lobo
    ('Troll_Zumbi', 2392,  50),        -- Âmago de Lobo
    ('Arq_Caveira', 2392,  50),        -- Âmago de Lobo
    ('Caveira_Lanc', 2392,  50),       -- Âmago de Lobo
    ('Conj_Caveira', 2392,  50),       -- Âmago de Lobo
    ('Hidra', 2394,  50),              -- Âmago de Urso
    ('Cav.Caveira', 2394,  50),        -- Âmago de Urso
    ('Hidra_Dourada_', 2394,  50),     -- Âmago de Urso
    ('Elfo_Negro', 2394,  50),         -- Âmago de Urso
    ('Guer_Caveira', 2394,  50),       -- Âmago de Urso
    ('Anf_Assassino', 2393,  50),      -- Âmago de Dragão Menor
    ('Bruxa_Elfica', 2393,  50),       -- Âmago de Dragão Menor
    ('Gargula_Sabio', 2393,  50),      -- Âmago de Dragão Menor
    ('Gargula', 2393,  50),            -- Âmago de Dragão Menor
    ('Golem_de_Fogo', 2393,  50),      -- Âmago de Dragão Menor
    ('Anf_Ninja', 2396,  33),          -- Âmago de Cav s/Sela N
    ('Anf_Ninja', 2401,  29),          -- Âmago de Ca s/Sela B
    ('Grim_Lock', 2396,  33),          -- Âmago de Cav s/Sela N
    ('Grim_Lock', 2401,  29),          -- Âmago de Ca s/Sela B
    ('Gargula_Inf', 2396,  33),        -- Âmago de Cav s/Sela N
    ('Gargula_Inf', 2401,  29),        -- Âmago de Ca s/Sela B
    ('Gargula_Servo', 2396,  33),      -- Âmago de Cav s/Sela N
    ('Gargula_Servo', 2401,  29),      -- Âmago de Ca s/Sela B
    ('Golem_de_Pedra', 2396,  33),     -- Âmago de Cav s/Sela N
    ('Golem_de_Pedra', 2401,  29),     -- Âmago de Ca s/Sela B
    ('Hezling', 2397,  33),            -- Âmago de Cav Fantasm N
    ('Hezling', 2402,  29),            -- Âmago de Cav Fantasm B
    ('Hezling', 2398,  33),            -- Âmago de Cavalo Leve N
    ('Hezling', 2403,  29),            -- Âmago de Cavalo Leve B
    ('Dragao_Lich', 2397,  33),        -- Âmago de Cav Fantasm N
    ('Dragao_Lich', 2402,  29),        -- Âmago de Cav Fantasm B
    ('Dragao_Lich', 2398,  33),        -- Âmago de Cavalo Leve N
    ('Dragao_Lich', 2403,  29),        -- Âmago de Cavalo Leve B
    ('Cav._Mortal', 2397,  33),        -- Âmago de Cav Fantasm N
    ('Cav._Mortal', 2402,  29),        -- Âmago de Cav Fantasm B
    ('Cav._Mortal', 2398,  33),        -- Âmago de Cavalo Leve N
    ('Cav._Mortal', 2403,  29),        -- Âmago de Cavalo Leve B
    ('Cavaleiro_Negro', 2397,  33),    -- Âmago de Cav Fantasm N
    ('Cavaleiro_Negro', 2402,  29),    -- Âmago de Cav Fantasm B
    ('Cavaleiro_Negro', 2398,  33),    -- Âmago de Cavalo Leve N
    ('Cavaleiro_Negro', 2403,  29),    -- Âmago de Cavalo Leve B
    ('Demon_Lord', 2397,  33),         -- Âmago de Cav Fantasm N
    ('Demon_Lord', 2402,  29),         -- Âmago de Cav Fantasm B
    ('Demon_Lord', 2398,  33),         -- Âmago de Cavalo Leve N
    ('Demon_Lord', 2403,  29)          -- Âmago de Cavalo Leve B
ON CONFLICT (mob, item) DO UPDATE SET chance = EXCLUDED.chance, updated_at = now();

UPDATE drop_rule_meta SET version = version + 1 WHERE id = TRUE;
