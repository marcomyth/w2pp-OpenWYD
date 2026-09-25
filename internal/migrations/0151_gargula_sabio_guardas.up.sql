-- 0151_gargula_sabio_guardas — o saque dos guardas da Gárgula Sábio do 2º andar da
-- Dungeon, no bloco da foto do Marco (25/09/2026, em (865,3879)).
--
-- Os guardas são os Golens de Fogo que nasciam como seguidores da Gárgula no bloco
-- 2540; agora nascem no bloco 6162, no mesmo ponto, com uma cópia do template
-- (Golem_Guarda) mais forte e de Carry vazio. A cópia existe para o saque valer só
-- ali: a Mesa vale por template, e o Golem de Fogo comum nasce no andar inteiro.
-- Então o que está aqui é TODO o saque deles:
--   Poeira de Oriharucon 2%, Poeira de Lactolerium 1%, cada uma das dez Jóias
--   (3200-3209) 0,2% e a Moeda de Prata (1Mi) 0,5%.
-- As chances foram escolha minha; o pedido não as deu. Pelo viés do sorteio da
-- Mesa (droprule/vies_test.go) cada número abaixo paga 22,1% a mais.
--
-- A Gárgula Sábio chefe (Gargula_Sabio_Chefe) não tem linha: o Carry dela é vazio
-- e a Arma C que ela solta é código (handler/gargula_sabio.go).
INSERT INTO drop_rule (mob, item, chance) VALUES
    ('Golem_Guarda',  412, 200),  -- Poeira de Oriharucon
    ('Golem_Guarda',  413, 100),  -- Poeira de Lactolerium
    ('Golem_Guarda', 3200,  20),  -- Jóia da Sagacidade
    ('Golem_Guarda', 3201,  20),  -- Jóia da Resistência
    ('Golem_Guarda', 3202,  20),  -- Jóia da Revelação
    ('Golem_Guarda', 3203,  20),  -- Jóia da Recuperação
    ('Golem_Guarda', 3204,  20),  -- Jóia da Absorção
    ('Golem_Guarda', 3205,  20),  -- Jóia da Proteção
    ('Golem_Guarda', 3206,  20),  -- Jóia do Poder
    ('Golem_Guarda', 3207,  20),  -- Jóia da Armazenagem
    ('Golem_Guarda', 3208,  20),  -- Jóia da Precisão
    ('Golem_Guarda', 3209,  20),  -- Jóia da Magia
    ('Golem_Guarda', 4026,  50)   -- Moeda de Prata(1Mi)
ON CONFLICT (mob, item) DO UPDATE SET chance = EXCLUDED.chance, updated_at = now();

UPDATE drop_rule_meta SET version = version + 1 WHERE id = TRUE;
