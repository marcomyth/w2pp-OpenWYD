-- 0152_sala_golem_de_fogo — o saque da sala do Golem de Fogo do 2º andar da
-- Dungeon, pedido do Marco em 25/09/2026 com print em (854,3931).
--
-- Só a sala da foto (x 841-868, y 3913-3941): os seis blocos dela usam cópias de
-- template de Carry vazio, Golem_Fogo_Lava e Gargula_Lava, porque a Mesa vale por
-- template e o Golem de Fogo e a Gárgula comuns nascem no andar inteiro. O que
-- está aqui é TODO o saque deles, o pedido:
--   âmagos de Sem Sela N 0,20% e B 0,15% (os mais difíceis, como na sala de lava),
--   os cinco brincos (591-595) a 0,10% cada — sai um deles ao acaso —, Resto de
--   Oriharucon 1% e de Lactolerium 0,5%, Classe D (a Repletion D) 0,5% e a Moeda
--   de Prata (1Mi) 0,3%.
-- As chances foram escolha minha; o pedido não as deu. Pelo viés do sorteio da
-- Mesa (droprule/vies_test.go) cada número abaixo paga 22,1% a mais.
--
-- O Boss Golem de Fogo (bloco 6163) não tem linha: o prêmio dele é código
-- (handler/dungeon_lava.go), UM por morte.
INSERT INTO drop_rule (mob, item, chance) VALUES
    ('Golem_Fogo_Lava', 2396,  20),  -- Âmago de Cav s/Sela N
    ('Golem_Fogo_Lava', 2401,  15),  -- Âmago de Ca s/Sela B
    ('Golem_Fogo_Lava',  591,  10),  -- Brinco de Athena
    ('Golem_Fogo_Lava',  592,  10),  -- Brinco de Titã
    ('Golem_Fogo_Lava',  593,  10),  -- Brinco de Zeus
    ('Golem_Fogo_Lava',  594,  10),  -- Brinco de Hecate
    ('Golem_Fogo_Lava',  595,  10),  -- Brinco de Hercules
    ('Golem_Fogo_Lava',  419, 100),  -- Resto de Oriharucon
    ('Golem_Fogo_Lava',  420,  50),  -- Resto de Lactolerium
    ('Golem_Fogo_Lava', 4019,  50),  -- Classe D
    ('Golem_Fogo_Lava', 4026,  30),  -- Moeda de Prata(1Mi)
    ('Gargula_Lava',    2396,  20),  -- Âmago de Cav s/Sela N
    ('Gargula_Lava',    2401,  15),  -- Âmago de Ca s/Sela B
    ('Gargula_Lava',     591,  10),  -- Brinco de Athena
    ('Gargula_Lava',     592,  10),  -- Brinco de Titã
    ('Gargula_Lava',     593,  10),  -- Brinco de Zeus
    ('Gargula_Lava',     594,  10),  -- Brinco de Hecate
    ('Gargula_Lava',     595,  10),  -- Brinco de Hercules
    ('Gargula_Lava',     419, 100),  -- Resto de Oriharucon
    ('Gargula_Lava',     420,  50),  -- Resto de Lactolerium
    ('Gargula_Lava',    4019,  50),  -- Classe D
    ('Gargula_Lava',    4026,  30)   -- Moeda de Prata(1Mi)
ON CONFLICT (mob, item) DO UPDATE SET chance = EXCLUDED.chance, updated_at = now();

UPDATE drop_rule_meta SET version = version + 1 WHERE id = TRUE;
