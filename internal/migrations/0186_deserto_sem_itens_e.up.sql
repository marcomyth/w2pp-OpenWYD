-- 0186_deserto_sem_itens_e — o Verme, o Aeon e o Adamant do Deserto deixam de
-- soltar armas E e peças do set E, como a Mantícora na 0183.
--
-- Pedido do Marco em 29/09/2026, depois de uma Armadura Legionária [E] cair de
-- Verme: a 0183 só tinha tirado os itens E da Mantícora, e os outros três
-- templates do Deserto que os carregam continuaram soltando pelo template. A
-- varredura dos onze templates da 0109 achou peças E só nestes quatro; Tauron,
-- Ladrão, Aranha, Assassino, Arqueiro, Treant e Lugefer não têm nenhuma.
--
--   Verme_          vagas 0-3   Karikas (3576) e Arco Guardião (3556), duas vagas
--                               cada, 1 em 900
--                   vagas 40-47 as armaduras dos quatro sets E, duas vagas cada,
--                               1 em 2.000
--   Aeon_Tauron     vagas 0-2   Cajado Caótico (3582), Arco Guardião (3556) e
--                               Dianus (3561), 1 em 900
--                   vagas 40-47 as calças dos quatro sets E
--   Adamant_Tauron  vagas 0-2   Demolidor Celestial (3596), Foice Platinada (3566)
--                               e Força Eterna (3581), 1 em 900
--                   vagas 40-47 as botas dos quatro sets E
--
-- Uma regra a 0% tira o item do template só daquele monstro, em todas as vagas
-- em que ele aparece (droprule.Governs). O resto da mesa do Deserto (Poeiras,
-- Moeda, âmagos, gemas, Classe C e D, Armas D da 0180) não muda.
INSERT INTO drop_rule (mob, item, chance) VALUES
    ('Verme_',         3576, 0),   -- Karikas
    ('Verme_',         3556, 0),   -- Arco Guardião
    ('Verme_',         1226, 0),   -- Armadura Mortal
    ('Verme_',         1361, 0),   -- Túnica Templária
    ('Verme_',         1511, 0),   -- Armadura do Corvo
    ('Verme_',         1661, 0),   -- Armadura Legionária
    ('Aeon_Tauron',    3582, 0),   -- Cajado Caótico
    ('Aeon_Tauron',    3556, 0),   -- Arco Guardião
    ('Aeon_Tauron',    3561, 0),   -- Dianus
    ('Aeon_Tauron',    1227, 0),   -- Calça Mortal
    ('Aeon_Tauron',    1362, 0),   -- Calça Templária
    ('Aeon_Tauron',    1512, 0),   -- Calça do Corvo
    ('Aeon_Tauron',    1662, 0),   -- Calça Legionária
    ('Adamant_Tauron', 3596, 0),   -- Demolidor Celestial
    ('Adamant_Tauron', 3566, 0),   -- Foice Platinada
    ('Adamant_Tauron', 3581, 0),   -- Força Eterna
    ('Adamant_Tauron', 1229, 0),   -- Botas Mortais
    ('Adamant_Tauron', 1364, 0),   -- Botas Templárias
    ('Adamant_Tauron', 1514, 0),   -- Botas do Corvo
    ('Adamant_Tauron', 1664, 0)    -- Botas Legionárias
ON CONFLICT (mob, item) DO UPDATE SET chance = EXCLUDED.chance, updated_at = now();

UPDATE drop_rule_meta SET version = version + 1 WHERE id = TRUE;
