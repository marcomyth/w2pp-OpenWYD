-- 0102_arenas_repletion_e_amagos — as três arenas de cima passam a pagar
-- Repletion, e os Âmagos que já caem lá dobram de chance.
--
-- Pedido do Marco em 22/09/2026: "Podemos no Kaizen, Hidra e Elfos, por
-- Repletion e Âmago de lobo, dragão menor, dente de Sabre e outros" — e, quando
-- perguntado, "só Classe C e D poucos" para o Repletion e "aumentar a chance"
-- para os Âmagos.
--
-- REPLETION, novo. Só as Classes C (4018) e D (4019), e de propósito longe do
-- que o Castelo Orc paga (9,5% e 6,5%, migração 0053): lá o Repletion é a moeda
-- da corrida, aqui é troco.
--
--                    Classe C 4018     Classe D 4019
--   líder             4%  (4,88%)       2%  (2,44%)
--   seguidor          2%  (2,44%)       1%  (1,22%)
--
-- O número entre parênteses é o que o jogo ENTREGA: o sorteio da Mesa é
-- rand()%10000 sobre um rand() que para em 32767 e infla toda chance abaixo de
-- 27,68% em 22,07% (internal/droprule/vies_test.go). Aqui os redondos ficaram,
-- porque "poucos" não pede casa decimal — mas quem for reajustar precisa saber
-- que o painel escreve 4% e o jogo paga 4,88%.
--
-- MEDIDO, por arena cheia limpa uma vez: 2,00 Repletion no Kaizen, 2,52 nas
-- Hidras e 2,28 nos Elfos, sempre com mais C que D.
--
-- ÂMAGOS, o dobro. Os quatro de entrada — Lobo 2392, Dragão Menor 2393, Urso
-- 2394 e Dente de Sabre 2395 — já caíam nas três arenas desde a 0065/0075, a 6%
-- no líder e 1,5% no seguidor. Passam a 12% e 3%.
--
-- ISSO ACELERA MONTARIA, e é a consequência a vigiar: o Âmago é o que alimenta a
-- curva de crescimento, que tem mesa própria no painel (/rates/montarias). Uma
-- arena cheia passa, MEDIDO, de 5,71 para 11,37 Âmagos no Kaizen, de 7,48 para
-- 14,70 nas Hidras e de 6,97 para 13,80 nos Elfos. Se a montaria começar a subir
-- rápido demais, é aqui que se mexe antes de mexer na curva.
--
-- (Os "~2,9 Âmagos" que a 0065 anunciou eram conta de papel sobre as chances
-- nominais; o jogo entregava 5,71, porque o viés acima não estava na conta.)
--
-- As chances dos Restos (0098) e da moeda (0099/0101) não são tocadas.

INSERT INTO drop_rule (mob, item, chance) VALUES
    -- Repletion: Classe C e D, o líder pagando o dobro do seguidor.
    ('Cav._Kaizen',   4018, 400), ('Cav._Servo',    4018, 200),
    ('Cav._Kaizen',   4019, 200), ('Cav._Servo',    4019, 100),
    ('Hidra_Dourada', 4018, 400), ('Hidra_Imortal', 4018, 200),
    ('Hidra_Dourada', 4019, 200), ('Hidra_Imortal', 4019, 100),
    ('Mestre_Elfo',   4018, 400), ('Servo_Elfo',    4018, 200),
    ('Mestre_Elfo',   4019, 200), ('Servo_Elfo',    4019, 100),
    -- Âmagos: o dobro do que a 0065/0075 escreveu.
    ('Cav._Kaizen',   2392, 1200), ('Cav._Servo',    2392, 300),
    ('Cav._Kaizen',   2393, 1200), ('Cav._Servo',    2393, 300),
    ('Cav._Kaizen',   2394, 1200), ('Cav._Servo',    2394, 300),
    ('Cav._Kaizen',   2395, 1200), ('Cav._Servo',    2395, 300),
    ('Hidra_Dourada', 2392, 1200), ('Hidra_Imortal', 2392, 300),
    ('Hidra_Dourada', 2393, 1200), ('Hidra_Imortal', 2393, 300),
    ('Hidra_Dourada', 2394, 1200), ('Hidra_Imortal', 2394, 300),
    ('Hidra_Dourada', 2395, 1200), ('Hidra_Imortal', 2395, 300),
    ('Mestre_Elfo',   2392, 1200), ('Servo_Elfo',    2392, 300),
    ('Mestre_Elfo',   2393, 1200), ('Servo_Elfo',    2393, 300),
    ('Mestre_Elfo',   2394, 1200), ('Servo_Elfo',    2394, 300),
    ('Mestre_Elfo',   2395, 1200), ('Servo_Elfo',    2395, 300)
ON CONFLICT (mob, item) DO UPDATE SET chance = EXCLUDED.chance, updated_at = now();

UPDATE drop_rule_meta SET version = version + 1 WHERE id = TRUE;
