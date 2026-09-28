-- 0176_hidras_chave_so_no_abate — a Chave do Rei Orc cai demais nas arenas das
-- Hidras e dos Elfos; em troca, as Hidras soltam o Ovo de Dente de Sabre e mais
-- Moeda de 5 milhões.
--
-- Pedido do Marco em 27/09/2026: "em 2 entradas Hidras dropei 4. Deveria dropar
-- 1", entrando com 4 personagens. Escolheu cortar o sorteio da entrada e deixar a
-- chave só no abate, e o mesmo corte nos Elfos. Depois: "nas hidras coloque a
-- chance de drop de ovo de dente de sabre e verifique o drop de Barra de ouro de
-- 1 ou 5KK para ajudar".
--
-- A CONTA DE ANTES. A entrada paga sorteava a chave a 1 em 4 (Hidras) e 1 em 3
-- (Elfos) POR PERSONAGEM que gastava o bilhete: um grupo de 4 levava ~1 chave só
-- de entrar. Esse sorteio sai do código no mesmo commit (castelo_orc.go). O abate
-- (0075) somava ~0,67 chave por entrada: uma entrada de 10 minutos mata ~37
-- Douradas e ~180 Imortais, porque o bloco 3498 renasce 15 s depois de morrer e
-- as Imortais soltas voltam a cada 2 minutos. Total ~1,7 por entrada, ~3,3 em
-- duas — os 4 do relato.
--
-- A CHAVE, só no abate, para dar ~0,5 por entrada (1 a cada 2 entradas):
--                                  antes (0075)    agora
--   Hidra Dourada, Mestre Elfo       0,5%           0,4%  (0,49% real)
--   Hidra Imortal, Servo Elfo        0,2%           0,15% (0,18% real)
--
-- NAS HIDRAS, como ajuda (as chances foram escolha minha; o pedido não as deu):
--   Ovo de Dente de Sabre 2305  Dourada 0,5%, Imortal 0,1%: ~0,45 ovo por
--     entrada. Nenhuma das duas o carregava no template.
--   Moeda de Prata (5Mi) 4027   Dourada 1%, Imortal 0,2%: ~0,9 moeda (~4,5
--     milhões) por entrada. Só a Imortal a soltava, pelo template (slots 48, 49
--     e 62, ~0,09% por morte, ~0,85 milhão por entrada); a regra passa a decidir
--     e o slot do template é pulado. A de 5 milhões e não a de 1 é a decisão da
--     0101: "Barra 5KK podemos dropar nas Hidras, Elfos, mas nas outras 1KK".
--
-- Pelo viés do sorteio da Mesa (droprule/vies_test.go) cada chance abaixo paga
-- 22,1% a mais; os números "por entrada" acima já contam com ele. A simulação
-- está em handler/hidras_entrada_test.go.
INSERT INTO drop_rule (mob, item, chance) VALUES
    ('Hidra_Dourada',  465,  40),  -- Chave do Rei Orc
    ('Hidra_Imortal',  465,  15),  -- Chave do Rei Orc
    ('Mestre_Elfo',    465,  40),  -- Chave do Rei Orc
    ('Servo_Elfo',     465,  15),  -- Chave do Rei Orc
    ('Hidra_Dourada', 2305,  50),  -- Ovo Dente de Sabre
    ('Hidra_Imortal', 2305,  10),  -- Ovo Dente de Sabre
    ('Hidra_Dourada', 4027, 100),  -- Moeda de Prata (5Mi)
    ('Hidra_Imortal', 4027,  20)   -- Moeda de Prata (5Mi)
ON CONFLICT (mob, item) DO UPDATE SET chance = EXCLUDED.chance, updated_at = now();

UPDATE drop_rule_meta SET version = version + 1 WHERE id = TRUE;
