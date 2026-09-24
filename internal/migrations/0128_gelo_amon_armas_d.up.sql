-- 0128_gelo_amon_armas_d — o Soldado e o Guerreiro Amon soltam o topo das Armas D.
--
-- Pedido da equipe em 24/09/2026. Os dois macacos fortes do Gelo ficaram sem
-- equipamento na 0110: tudo o que soltavam era Arch, e o Arch saiu da área. Agora
-- soltam as nove Armas D de topo que os lobos, o Urso Polar e o Ent já soltam
-- (Divino, Solaris, Vorpal e as outras), a 0,1% cada — o dobro dos lobos, como os
-- Kalintz soltam as armas E. Com o viés do rand() do MSVC abaixo de 27,68%, 10
-- paga 0,12% (internal/droprule/vies_test.go): ~1,1% dos abates dá uma das nove.
--
-- O add é código (handler/gelo.go, geloAmonFinish): sorteado numa escada que
-- chega a 45-54 de dano nas físicas e 20-24 de magia na Lança do Triunfo e na
-- Fúria Divina. Os dois templates só nascem no Gelo (12 Soldados, 19 Guerreiros).
INSERT INTO drop_rule (mob, item, chance) VALUES
    ('Soldado_Amon',    810,   10),  -- Martelo Assassino
    ('Soldado_Amon',    825,   10),  -- Arco Divino
    ('Soldado_Amon',    840,   10),  -- Garra Draconiana
    ('Soldado_Amon',    855,   10),  -- Lança do Triunfo
    ('Soldado_Amon',    870,   10),  -- Espada Vorpal
    ('Soldado_Amon',    885,   10),  -- Cruz Sagrada
    ('Soldado_Amon',    900,   10),  -- Fúria Divina
    ('Soldado_Amon',    911,   10),  -- Solaris
    ('Soldado_Amon',    936,   10),  -- Mjolnir
    ('Guerreiro_Amon',  810,   10),  -- Martelo Assassino
    ('Guerreiro_Amon',  825,   10),  -- Arco Divino
    ('Guerreiro_Amon',  840,   10),  -- Garra Draconiana
    ('Guerreiro_Amon',  855,   10),  -- Lança do Triunfo
    ('Guerreiro_Amon',  870,   10),  -- Espada Vorpal
    ('Guerreiro_Amon',  885,   10),  -- Cruz Sagrada
    ('Guerreiro_Amon',  900,   10),  -- Fúria Divina
    ('Guerreiro_Amon',  911,   10),  -- Solaris
    ('Guerreiro_Amon',  936,   10)   -- Mjolnir
ON CONFLICT (mob, item) DO UPDATE SET chance = EXCLUDED.chance, updated_at = now();

UPDATE drop_rule_meta SET version = version + 1 WHERE id = TRUE;
