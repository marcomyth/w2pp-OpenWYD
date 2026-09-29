-- 0186_deserto_e_vira_d — no Deserto, arma E e set E viram Arma D e Set D (A).
--
-- Pedido do Marco em 29/09/2026, depois de uma Armadura Legionária [E] cair de
-- Verme: "Sets E e Armas E só no gelo e Kefra. Retire do Deserto troque por Set
-- D A e Armas D com adds razoáveis e randômicos." A letra do tooltip é o
-- EF_ITEMLEVEL: 5 é E, 4 é D. O (A) é a versão Épica da peça (Grade 3).
--
-- A varredura dos onze templates do Deserto (0109) achou itens E em quatro:
-- Verme, Mantícora, Aeon e Adamant. A 0183 já zerou os da Mantícora; esta zera
-- os dos outros três e põe, nos quatro, o equivalente D pela Mesa:
--
--   arma E -> Arma D do mesmo tipo, com o add do Deserto (handler/deserto_armas.go):
--     dano 45-72 ou, nas que têm EF_MAGIC, magia 20-32
--   peça E -> a mesma peça do Set D (A) das quatro classes: Embutido (TK), Mytril
--     (FM), Elemental (BM) e Teia (HT). O add vem do bônus de drop comum, com o
--     teto dos adds de armadura (rolarBonusDrop).
--
-- As chances repetem o que o item E pagava pelo template a quem mata sem bônus:
--   arma E numa vaga de 1 em 900 (0,11%)  -> 9 aqui (paga 0,110%); no Verme cada
--     arma E estava em duas vagas (0,22%) -> 18 (paga 0,220%)
--   peça E em duas vagas de 1 em 2.000 (0,10%) -> 8 aqui (paga 0,098%)
-- A Mesa sorteia rand() % 10000 sobre o rand() de 15 bits do MSVC e, abaixo de
-- 27,68%, paga 4/3,2768 do escrito (internal/droprule/vies_test.go). O sorteio da
-- Mesa não olha o bônus de drop de quem mata; o do template olhava.
--
-- Uma regra a 0% tira o item do template só daquele monstro, em todas as vagas
-- em que ele aparece (droprule.Governs). O resto da mesa do Deserto (Poeiras,
-- Moeda, âmagos, gemas, Classe C e D, as Armas D da 0180) não muda.
INSERT INTO drop_rule (mob, item, chance) VALUES
    -- Verme: Karikas e Arco Guardião; as armaduras dos sets E.
    ('Verme_',         3576, 0),   -- Karikas
    ('Verme_',         3556, 0),   -- Arco Guardião
    ('Verme_',         1226, 0),   -- Armadura Mortal
    ('Verme_',         1361, 0),   -- Túnica Templária
    ('Verme_',         1511, 0),   -- Armadura do Corvo
    ('Verme_',         1661, 0),   -- Armadura Legionária
    ('Verme_',          884, 18),  -- Lança Relâmpago     (no lugar da Karikas)
    ('Verme_',          825, 18),  -- Arco Divino         (no lugar do Arco Guardião)
    ('Verme_',         1211, 8),   -- Armadura Embutida(A)
    ('Verme_',         1346, 8),   -- Túnica de Mytril(A)
    ('Verme_',         1496, 8),   -- Armadura Elemental(A)
    ('Verme_',         1646, 8),   -- Peitoral de Teia(A)

    -- Mantícora: as armas E e os elmos E já saíram na 0183.
    ('Manticora',       869, 9),   -- Gram                (no lugar da Vingadora)
    ('Manticora',       910, 9),   -- Luna                (no lugar da Éden)
    ('Manticora',       809, 9),   -- Martelo Dragão      (no lugar da Asa Draconiana)
    ('Manticora',      1208, 8),   -- Elmo Embutido(A)
    ('Manticora',      1343, 8),   -- Chapéu de Mytril(A)
    ('Manticora',      1493, 8),   -- Elmo Elemental(A)
    ('Manticora',      1643, 8),   -- Chapéu de Teia(A)

    -- Aeon: Cajado Caótico, Arco Guardião e Dianus; as calças dos sets E.
    ('Aeon_Tauron',    3582, 0),   -- Cajado Caótico
    ('Aeon_Tauron',    3556, 0),   -- Arco Guardião
    ('Aeon_Tauron',    3561, 0),   -- Dianus
    ('Aeon_Tauron',    1227, 0),   -- Calça Mortal
    ('Aeon_Tauron',    1362, 0),   -- Calça Templária
    ('Aeon_Tauron',    1512, 0),   -- Calça do Corvo
    ('Aeon_Tauron',    1662, 0),   -- Calça Legionária
    ('Aeon_Tauron',     900, 9),   -- Fúria Divina        (no lugar do Cajado Caótico)
    ('Aeon_Tauron',     825, 9),   -- Arco Divino         (no lugar do Arco Guardião)
    ('Aeon_Tauron',     840, 9),   -- Garra Draconiana    (no lugar da Dianus)
    ('Aeon_Tauron',    1214, 8),   -- Calça Embutida(A)
    ('Aeon_Tauron',    1349, 8),   -- Calça de Mytril(A)
    ('Aeon_Tauron',    1499, 8),   -- Calça Elemental(A)
    ('Aeon_Tauron',    1649, 8),   -- Calça de Teia(A)

    -- Adamant: Demolidor Celestial, Foice Platinada e Força Eterna; as botas E.
    ('Adamant_Tauron', 3596, 0),   -- Demolidor Celestial
    ('Adamant_Tauron', 3566, 0),   -- Foice Platinada
    ('Adamant_Tauron', 3581, 0),   -- Força Eterna
    ('Adamant_Tauron', 1229, 0),   -- Botas Mortais
    ('Adamant_Tauron', 1364, 0),   -- Botas Templárias
    ('Adamant_Tauron', 1514, 0),   -- Botas do Corvo
    ('Adamant_Tauron', 1664, 0),   -- Botas Legionárias
    ('Adamant_Tauron',  935, 9),   -- Martelo Psíquico    (no lugar do Demolidor Celestial)
    ('Adamant_Tauron',  854, 9),   -- Gungnir             (no lugar da Foice Platinada)
    ('Adamant_Tauron',  900, 9),   -- Fúria Divina        (no lugar da Força Eterna)
    ('Adamant_Tauron', 1220, 8),   -- Botas Embutidas(A)
    ('Adamant_Tauron', 1355, 8),   -- Botas de Mytril(A)
    ('Adamant_Tauron', 1505, 8),   -- Botas Elementais(A)
    ('Adamant_Tauron', 1655, 8)    -- Botas de Teia(A)
ON CONFLICT (mob, item) DO UPDATE SET chance = EXCLUDED.chance, updated_at = now();

UPDATE drop_rule_meta SET version = version + 1 WHERE id = TRUE;
