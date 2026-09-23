-- 0099_aparicao_moeda_de_1kk — a Aparição passa a soltar a Moeda de 1 milhão no
-- lugar da de 5, na mesma frequência.
--
-- Pedido do Marco em 22/09/2026: "ta dropando muita barra de 5KK; podemos deixar
-- dropar na mesma frequência mas a de 1KK e não a de 5KK — dropou 20 em menos de
-- 30 minutos, ou seja 100KK ou mais por 30 minutos; com isso a economia vai pro
-- saco".
--
-- A fonte é UMA só, e não a Mesa: o template da Aparição (Release/TMsrv/run/npc)
-- carrega a Moeda de Prata (5Mi) 4027 no slot 57, e esse slot tem droprate 35 —
-- 2,857% por morte, de longe o maior drop de barra do conteúdo inteiro. Todos os
-- outros monstros que soltam 4027 ficam entre 0,03% e 0,05%:
--
--   Aparicao        slot 57    2,8571%   <-- esta linha
--   Cav._Servo      slots 40-44  0,05% cada
--   Servo_Elfo      slots 40-43  0,05% cada
--   Hidra_Imortal   slots 48,49,62  0,03% cada
--
-- A Aparição é a líder da arena do Cemitério (a quest da caveira), 29 numa arena
-- cheia, e morre em um golpe para quem está na faixa. Daí as 20 em meia hora.
--
-- A troca é feita pela Mesa e não editando o template, porque o binário do
-- template é conteúdo do legado e mexer nele sai do alcance do pedido: a regra
-- ('Aparicao', 4027, 0) faz o laço de drop PULAR o slot 57 (droprule.Governs em
-- mobkilled.go) e sortear 0% no lugar, e a linha da 4026 põe a moeda de 1 milhão
-- com a mesma frequência.
--
-- POR QUE 234 E NÃO 286: o sorteio da Mesa é rand()%10000 sobre um rand() do
-- MSVC que para em 32767, e isso infla toda chance abaixo de 27,68% em 22,07%
-- (internal/droprule/vies_test.go). Para ENTREGAR os mesmos 2,857% do slot 57 é
-- preciso escrever 2,34%: 234 x 4 / 32768 = 2,856%. Escrever 286 entregaria
-- 3,49% e a troca teria aumentado a frequência em vez de mantê-la.
--
-- O que isso faz com o ouro do Cemitério: 29 Aparições x 2,857% = 0,83 moeda por
-- arena limpa, que era 4,14 milhões e passa a ser 828 mil. Um quinto.
--
-- As outras quatro linhas de 4027 ficam como estão: não foram pedidas, e a 0,05%
-- elas não movem economia nenhuma.

INSERT INTO drop_rule (mob, item, chance) VALUES
    ('Aparicao', 4027,   0),
    ('Aparicao', 4026, 234)
ON CONFLICT (mob, item) DO UPDATE SET chance = EXCLUDED.chance, updated_at = now();

UPDATE drop_rule_meta SET version = version + 1 WHERE id = TRUE;
