-- 0169_aqua_golem_submundo — o saque do Aqua Golem do Submundo (pedido de
-- 26/09/2026). Só o Aqua_Golem de campo aberto: os da Água (Aqua_Golem_ e
-- Aqua_Golem___) têm outro template_name e esta mesa não os alcança.
--
-- Chances em centésimos de por cento, como a 0090 e o painel gravam. Uma regra
-- SUBSTITUI o que o template faria para aquele par (monstro, item).
--
--   item  nome                      chance        1 a cada
--   419   Resto de Oriharucon       3%            33 abates
--   420   Resto de Lactolerium      1,5%          67
--   4042  Emblema do Guarda         1%            100      (ingresso da arena dos Elfos, Quest 256)
--   4019  Repletion D (Classe D)    0,5%          200      (a régua da 0090 para Repletion)
--   4026  Moeda de Prata (1Mi)      0,2%          500      (vale 1.000.000 de ouro)
--   2307  Ovo de Cavalo Fantasma N  0             —        (saía a 0,024% do template)
--   2312  Ovo de Cavalo Fantasma B  0             —        (saía a 0,049% do template)
--
-- Continuam como estavam: os sets (A) do template — oito peças a 0,113% e quatro
-- luvas a 0,052% —, a Poeira de Lactolerium do template, e as regras da 0090
-- (quatro âmagos, Pergaminho da Água (N) e Classe C).
--
-- O Resto agora vende por unidade no NPC (60.000 o de Oriharucon, 100.000 o de
-- Lactolerium). Cem golems rendem em média 3 Restos de Oriharucon (180 mil de
-- ouro), 1,5 de Lactolerium (150 mil) e 0,2 Moeda (200 mil): perto de 530 mil de
-- ouro por cem abates, ou 650 mil com o desvio do rand() logo abaixo.
--
-- A Mesa rola com rand() % 10000, e o rand() do MSVC vai só até 32.767: abaixo
-- de 27,68% toda chance sai cerca de 22% acima do número gravado (3% vira ~3,7%).
--
-- A quantidade de golems triplica no mesmo commit, no NPCGener.txt: os 35 blocos
-- ativos passam de 1 golem para um trio (MaxNumMob 3, grupo de 2 seguidores).
INSERT INTO drop_rule (mob, item, chance) VALUES
    ('Aqua_Golem',  419, 300),   -- Resto de Oriharucon
    ('Aqua_Golem',  420, 150),   -- Resto de Lactolerium
    ('Aqua_Golem', 4042, 100),   -- Emblema do Guarda
    ('Aqua_Golem', 4019,  50),   -- Repletion D
    ('Aqua_Golem', 4026,  20),   -- Moeda de Prata (1Mi)
    ('Aqua_Golem', 2307,   0),   -- Ovo de Cavalo Fantasma N: sai
    ('Aqua_Golem', 2312,   0)    -- Ovo de Cavalo Fantasma B: sai
ON CONFLICT (mob, item) DO UPDATE SET chance = EXCLUDED.chance, updated_at = now();

UPDATE drop_rule_meta SET version = version + 1 WHERE id = TRUE;
