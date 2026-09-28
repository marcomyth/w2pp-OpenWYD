-- 0144_dungeon_caveiras_restos_e_armas_c — o spot de Caveira Lanc e Conj Caveira
-- do 1º andar da Dungeon solta Restos e as Armas C.
--
-- Pedido do Marco em 25/09/2026, com prints em (353,3756). No NPCGener.txt do
-- mesmo commit os blocos dos dois passam a MaxNumMob x5 (Caveira Lanc 45 -> 225,
-- Conj Caveira 30 -> 150) e o Boss Conjurador entra no bloco 6149.
--
-- Os drops são SOMADOS: o que o template já soltava continua. Nos dois, a Mesa
-- passa a rolar:
--   Resto de Oriharucon 419     1%    (100)
--   Resto de Lactolerium 420    0,5%  (50)
--   as 19 Armas C               0,05% (5) cada
-- O pedido não deu chance; estes são o ponto de partida, ajustável na tela
-- /drops. Com o viés do rand() do MSVC (internal/droprule/vies_test.go) as 19
-- armas juntas pagam ~1,2% dos abates.
--
-- O add de cada arma é código (handler/dungeon_caveiras.go, caveirasFinish):
-- 45, 54 ou 63 de dano nas físicas; 20, 24 ou 28 de magia nas lanças e cajados.
--
-- Uma regra da Mesa vale para o template inteiro. Os dois só nascem no 1º andar
-- da Dungeon, fora dois Conj Caveira seguidores de Elfo Negro (blocos 2097 e
-- 2098), que também passam a soltar isto.
INSERT INTO drop_rule (mob, item, chance) VALUES
    ('Caveira_Lanc', 419, 100),  -- Resto de Oriharucon
    ('Caveira_Lanc', 420,  50),  -- Resto de Lactolerium
    ('Caveira_Lanc', 807,   5),  -- Maça Gótica (física)
    ('Caveira_Lanc', 808,   5),  -- Martelo Mythril (física)
    ('Caveira_Lanc', 822,   5),  -- Arco Mythril (física)
    ('Caveira_Lanc', 823,   5),  -- Arco Hidra (física)
    ('Caveira_Lanc', 837,   5),  -- Lança Invisível (física)
    ('Caveira_Lanc', 838,   5),  -- Faca do Assassino (física)
    ('Caveira_Lanc', 867,   5),  -- Gladio (física)
    ('Caveira_Lanc', 868,   5),  -- Lâmina Espiritual (física)
    ('Caveira_Lanc', 882,   5),  -- Shamir (física)
    ('Caveira_Lanc', 883,   5),  -- Faca Astaroth (física)
    ('Caveira_Lanc', 908,   5),  -- Espada Larga (física)
    ('Caveira_Lanc', 909,   5),  -- Lâmina Dupla (física)
    ('Caveira_Lanc', 933,   5),  -- Machado de Batalha (física)
    ('Caveira_Lanc', 934,   5),  -- Grande Machado (física)
    ('Caveira_Lanc', 852,   5),  -- Lança da Corrupção (mágica)
    ('Caveira_Lanc', 853,   5),  -- Longinius (mágica)
    ('Caveira_Lanc', 897,   5),  -- Cajado do Santo (mágica)
    ('Caveira_Lanc', 898,   5),  -- Varinha Alada (mágica)
    ('Caveira_Lanc', 901,   5),  -- Cajado da Jóia Azul (mágica)
    ('Conj_Caveira', 419, 100),  -- Resto de Oriharucon
    ('Conj_Caveira', 420,  50),  -- Resto de Lactolerium
    ('Conj_Caveira', 807,   5),  -- Maça Gótica (física)
    ('Conj_Caveira', 808,   5),  -- Martelo Mythril (física)
    ('Conj_Caveira', 822,   5),  -- Arco Mythril (física)
    ('Conj_Caveira', 823,   5),  -- Arco Hidra (física)
    ('Conj_Caveira', 837,   5),  -- Lança Invisível (física)
    ('Conj_Caveira', 838,   5),  -- Faca do Assassino (física)
    ('Conj_Caveira', 867,   5),  -- Gladio (física)
    ('Conj_Caveira', 868,   5),  -- Lâmina Espiritual (física)
    ('Conj_Caveira', 882,   5),  -- Shamir (física)
    ('Conj_Caveira', 883,   5),  -- Faca Astaroth (física)
    ('Conj_Caveira', 908,   5),  -- Espada Larga (física)
    ('Conj_Caveira', 909,   5),  -- Lâmina Dupla (física)
    ('Conj_Caveira', 933,   5),  -- Machado de Batalha (física)
    ('Conj_Caveira', 934,   5),  -- Grande Machado (física)
    ('Conj_Caveira', 852,   5),  -- Lança da Corrupção (mágica)
    ('Conj_Caveira', 853,   5),  -- Longinius (mágica)
    ('Conj_Caveira', 897,   5),  -- Cajado do Santo (mágica)
    ('Conj_Caveira', 898,   5),  -- Varinha Alada (mágica)
    ('Conj_Caveira', 901,   5)   -- Cajado da Jóia Azul (mágica)
ON CONFLICT (mob, item) DO UPDATE SET chance = EXCLUDED.chance, updated_at = now();

UPDATE drop_rule_meta SET version = version + 1 WHERE id = TRUE;
