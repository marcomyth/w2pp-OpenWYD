-- 0075_arena_elfos_e_chave_orc — a arena dos Elfos (Quest 256, passo 5, nível
-- 320-350) paga como a das Hidras, e a Chave do Rei Orc cai nas duas.
--
-- Pedido do Marco em 17/09/2026: "a quest dos elfos não tá funcionando, os itens
-- não dropam conforme as outras; ajuste a conf dela igual à das Hidras, e adicione
-- a chave orc para dropar nelas".
--
-- Os Elfos ficaram de fora da 0065: o Mestre Elfo soltava os Restos só pelos
-- slots 8 e 9 do template (25% cada, uma unidade) e nenhum Âmago além do Dente de
-- Sabre a 1 em 891; o Servo Elfo, nenhum Resto e nenhum Âmago. Aqui eles recebem
-- os números das Hidras, papel por papel — o Mestre é o líder (Hidra Dourada), o
-- Servo segue (Hidra Imortal). O pacote do Mestre (3 Oriharucon, 2 Lactolerium) é
-- do tmServer, handler/arenas_quest256.go.
--
--                               Mestre Elfo  Servo Elfo  (= Dourada / Imortal)
--   Resto de Oriharucon 419     50% (x3)     10%
--   Resto de Lactolerium 420    30% (x2)      5%
--   Cada Âmago, 2392 a 2395      6%           1,5%
--
-- A Chave do Rei Orc (465) sai do '*' a 0% da 0053 só nestes quatro monstros:
--
--   Hidra Dourada, Mestre Elfo (líderes)   0,5%
--   Hidra Imortal, Servo Elfo (seguidores) 0,2%
--
-- Uma arena das Hidras limpa (11 Douradas, 40 Imortais) dá ~0,14 chave; a dos
-- Elfos, com um Mestre por quatro, ~0,1. A chave sorteada na entrada paga (1 em 4
-- nas Hidras, 1 em 3 nos Elfos) continua. Ajuste fino na tela Drops do painel.
--
-- ON CONFLICT DO NOTHING, como na 0065: uma linha gravada pelo painel antes do
-- deploy vale mais que a proposta.

INSERT INTO drop_rule (mob, item, chance) VALUES
    -- Elfos: o Mestre lidera, o Servo segue.
    ('Mestre_Elfo',  419, 5000), ('Servo_Elfo',  419, 1000),
    ('Mestre_Elfo',  420, 3000), ('Servo_Elfo',  420,  500),
    ('Mestre_Elfo', 2392,  600), ('Servo_Elfo', 2392,  150),
    ('Mestre_Elfo', 2393,  600), ('Servo_Elfo', 2393,  150),
    ('Mestre_Elfo', 2394,  600), ('Servo_Elfo', 2394,  150),
    ('Mestre_Elfo', 2395,  600), ('Servo_Elfo', 2395,  150),
    -- A Chave do Rei Orc nas duas arenas.
    ('Hidra_Dourada', 465, 50), ('Hidra_Imortal', 465, 20),
    ('Mestre_Elfo',   465, 50), ('Servo_Elfo',    465, 20)
ON CONFLICT (mob, item) DO NOTHING;

-- O tmServer relê a mesa quando a versão muda.
UPDATE drop_rule_meta SET version = version + 1 WHERE id = TRUE;
