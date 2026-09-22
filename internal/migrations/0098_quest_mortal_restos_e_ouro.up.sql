-- 0098_quest_mortal_restos_e_ouro — as quests do Mortal pagam metade dos Restos
-- e 30% do ouro.
--
-- Pedido do Marco em 22/09/2026, depois de ver a bolsa voltar da arena cheia de
-- Restos: "os restos podemos diminuir 50% do drop e o gold em 70%, é muito
-- gold", e "na quest dos kaizen tá caindo mais restos que coração e isso tá
-- errado".
--
-- A MEDIÇÃO que motivou os números (handler/medicao_arenas_test.go, 2000 mortes
-- por template pelo caminho real de morte, arena cheia limpa uma vez):
--
--                      troféus   Restos   Restos por troféu
--   Cemitério            60,4     14,7          0,24
--   Jardim dos Deuses    59,2     13,9          0,23
--   Coração do Kaizen    27,2     36,9          1,36   <-- a reclamação
--   Hidras               33,4     45,5          1,36
--   Elfos                30,6     44,4          1,45
--
-- A causa do salto é a 0065/0075, não o legado: Cemitério e Jardim nunca
-- passaram pela Mesa e o líder deles solta meio Resto (slots 8 e 9 do template,
-- 25% cada, uma unidade). O Cav. Kaizen, desde 14/09, solta 50% x3 mais 30% x2 —
-- duas Restos e pouco contra um Coração. Cortar a chance pela metade devolve a
-- ordem certa: o troféu volta a cair mais que o Resto nas três arenas.
--
-- Do nível 39 ao 320, o Mortal tira das quests ~3.000 troféus, ~3.200 Restos e
-- 698 milhões de ouro — 454 do troféu, 236 de vender os Restos no NPC (o
-- Oriharucon vale 60.000 e o Lactolerium 100.000 na conta do sell) e 8 do resto
-- do saque. Depois desta migração são ~136 milhões do troféu e metade dos
-- Restos.
--
-- ALCANCE: só os seis monstros das três arenas que a Mesa governa. O Cemitério e
-- o Jardim ficam como estão — em 0,24 Resto por troféu eles não são o problema, e
-- cortá-los exigiria criar regra de Mesa onde nunca houve. O Acampamento Troll
-- (0071) e o Castelo Orc (0053) também soltam Restos e não foram pedidos.
--
-- O corte é relativo (chance / 2) e não absoluto, para que uma linha já ajustada
-- pelo painel seja cortada a partir do valor dela, e não substituída pela nossa.
-- O VALUES só vale para a linha que não existir.
--
-- ATENÇÃO, medido e NÃO corrigido aqui: o sorteio da Mesa é rand()%10000 e o
-- rand() do MSVC só vai até 32767, então toda chance abaixo de 27,68% sai 22,1%
-- maior do que o painel escreve (50% saem 54,2%; 15%, 18,3%). Isso vale para as
-- 619 regras da Mesa, não só para estas, e consertar é decisão à parte
-- (internal/droprule/vies_test.go mede). Por isso o corte de 50% na chance vira
-- ~44% no líder e 50% no seguidor: o viés é maior embaixo.

INSERT INTO drop_rule (mob, item, chance) VALUES
    -- Coração do Kaizen
    ('Cav._Kaizen',    419, 2500), ('Cav._Servo',    419,  750),
    ('Cav._Kaizen',    420, 1500), ('Cav._Servo',    420,  400),
    -- Hidras
    ('Hidra_Dourada',  419, 2500), ('Hidra_Imortal', 419,  500),
    ('Hidra_Dourada',  420, 1500), ('Hidra_Imortal', 420,  250),
    -- Elfos
    ('Mestre_Elfo',    419, 2500), ('Servo_Elfo',    419,  500),
    ('Mestre_Elfo',    420, 1500), ('Servo_Elfo',    420,  250)
ON CONFLICT (mob, item) DO UPDATE SET chance = drop_rule.chance / 2, updated_at = now();

UPDATE drop_rule_meta SET version = version + 1 WHERE id = TRUE;

-- O ouro dos cinco troféus a 30%. A tabela nasceu vazia na 0036, então até aqui
-- valiam os números de Common/Settings/QuestsRate.txt; o VALUES os repete, com o
-- Coin cortado, e a XP e as faixas ficam exatamente como estavam. Numa linha que
-- o painel já tenha gravado, só o Coin é cortado — e a partir do valor dela.
INSERT INTO quest_reward (tier, mortal_exp, arch_exp, coin, mortal_min, mortal_max, arch_min, arch_max) VALUES
    (0,  30000,  30000,   3000,  39, 115,  39, 115),
    (1,  60000,  60000,   6000, 115, 190, 115, 190),
    (2, 200000, 200000,  30000, 190, 265, 190, 265),
    (3, 500000, 500000,  75000, 265, 320, 265, 320),
    (4, 780000, 780000, 150000, 320, 350, 320, 350)
ON CONFLICT (tier) DO UPDATE SET coin = quest_reward.coin * 3 / 10, updated_at = now();

UPDATE quest_reward_meta SET version = version + 1 WHERE id = TRUE;
