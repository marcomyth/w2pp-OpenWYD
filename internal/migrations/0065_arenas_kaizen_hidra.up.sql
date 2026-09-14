-- 0065_arenas_kaizen_hidra — Restos e Âmagos nas arenas da Quest 256 do
-- Coração do Kaizen (passo 3) e das Hidras (passo 4).
--
-- Pedido da equipe em 14/09/2026: a arena do Kaizen estava pesada para quem
-- chega com os itens da quest (a vida dos dois monstros caiu pela metade no
-- template, Release/TMsrv/run/npc), e as duas arenas passam a pagar em Restos de
-- Oriharucon e de Lactolerium e em Âmagos das quatro montarias de entrada: Lobo
-- 2392, Dragão Menor 2393, Urso 2394 e Dente de Sabre 2395.
--
-- Uma arena cheia tem 6 Cav. Kaizen e 24 Cav. Servo; a das Hidras, 11 Hidras
-- Douradas e 40 Hidras Imortais (NPCGener 3476-3508). Chance em centésimos de
-- por cento; o pacote do líder de cada grupo é do tmServer
-- (handler/arenas_quest256.go), porque a Mesa não guarda quantidade:
--
--                               Cav. Kaizen  Cav. Servo  H. Dourada  H. Imortal
--   Resto de Oriharucon 419     50% (x3)     15%         50% (x3)    10%
--   Resto de Lactolerium 420    30% (x2)      8%         30% (x2)     5%
--   Cada Âmago, 2392 a 2395      6%           1,5%        6%          1,5%
--
-- Por arena limpa: Kaizen ~12,6 Oriharucon, ~5,5 Lactolerium e ~2,9 Âmagos;
-- Hidras ~20,5 Oriharucon, ~8,6 Lactolerium e ~5 Âmagos.
--
-- A regra toma o lugar do slot do template com o mesmo item: o Cav. Kaizen
-- soltava 25% de cada Resto pelos slots 8 e 9, e a Hidra Dourada 0,05% de Âmago
-- de Urso. Os troféus (4119, 4120) e o resto do saque ficam como estão.
--
-- ON CONFLICT DO NOTHING, como na 0053: uma linha gravada pelo painel antes do
-- deploy vale mais que a proposta.

INSERT INTO drop_rule (mob, item, chance) VALUES
    -- Coração do Kaizen: o líder e o seguidor.
    ('Cav._Kaizen',    419, 5000), ('Cav._Servo',    419, 1500),
    ('Cav._Kaizen',    420, 3000), ('Cav._Servo',    420,  800),
    ('Cav._Kaizen',   2392,  600), ('Cav._Servo',   2392,  150),
    ('Cav._Kaizen',   2393,  600), ('Cav._Servo',   2393,  150),
    ('Cav._Kaizen',   2394,  600), ('Cav._Servo',   2394,  150),
    ('Cav._Kaizen',   2395,  600), ('Cav._Servo',   2395,  150),
    -- Hidras: a Dourada lidera, a Imortal segue.
    ('Hidra_Dourada',  419, 5000), ('Hidra_Imortal',  419, 1000),
    ('Hidra_Dourada',  420, 3000), ('Hidra_Imortal',  420,  500),
    ('Hidra_Dourada', 2392,  600), ('Hidra_Imortal', 2392,  150),
    ('Hidra_Dourada', 2393,  600), ('Hidra_Imortal', 2393,  150),
    ('Hidra_Dourada', 2394,  600), ('Hidra_Imortal', 2394,  150),
    ('Hidra_Dourada', 2395,  600), ('Hidra_Imortal', 2395,  150)
ON CONFLICT (mob, item) DO NOTHING;

-- O tmServer relê a mesa quando a versão muda.
UPDATE drop_rule_meta SET version = version + 1 WHERE id = TRUE;
