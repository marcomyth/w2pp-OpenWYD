-- 0179_emblema_do_guarda_sessenta — o Emblema do Guarda (4042), a entrada da
-- arena dos Elfos na Quest 256, passa a cair 60% do que caía, em todo monstro
-- que nasce (pedido do Marco em 28/09/2026: "está dropando muito fácil").
--
-- São sete fontes. Três já são regras da Mesa e só mudam de número:
--
--   mob             era  fica  de onde vinha a regra
--   Aqua_Golem      100    60   0169
--   Argos            50    30   painel (nenhuma migração cria)
--   Argos_Errante    50    30   painel (nenhuma migração cria)
--
-- As outras quatro soltam pela vaga do Carry do template, várias vagas cada, e
-- ganham aqui uma regra nomeada. A regra toma o lugar de TODAS as vagas do
-- template com o item e rola uma vez só, então o número gravado é a chance
-- somada das vagas, vezes 0,6:
--
--   mob              vagas                   hoje     alvo     grava  paga
--   Elfo_Negro_Abj   4-7, 24, 32, 40         0,606%   0,364%   30     0,366%
--   Cav._Elfo_Negro  15, 24, 32, 40          0,268%   0,161%   13     0,159%
--   CH_Troll_Ghoul   21, 22, 23              0,018%   0,011%    1     0,012%
--   Troll_Ghoul      21, 22, 23              0,018%   0,011%    1     0,012%
--
-- "Hoje" é sem bônus de drop do matador. A vaga do template sobe com o bônus;
-- a regra da Mesa não. Para quem caça com bônus o corte nesses quatro é um pouco
-- maior que 40%.
--
-- A Mesa sorteia rand() % 10000 sobre o rand() de 15 bits do MSVC, e abaixo de
-- 27,68% paga 4/3,2768 do escrito. Nas três primeiras o desvio é o mesmo antes e
-- depois, então 60% do número escrito é 60% do real. Nas quatro últimas o número
-- já foi escolhido pelo que a Mesa PAGA, não pelo que está escrito. O 1 dos
-- Trolls é o menor que a Mesa grava; sai a 67% do atual, e não a 60%.
--
-- Os outros templates com 4042 no Carry (DarkElfAbjurer, Moloch, os PerGagoil…)
-- não têm bloco no NPCGener e não nascem.
INSERT INTO drop_rule (mob, item, chance) VALUES
    ('Aqua_Golem',      4042, 60),
    ('Argos',           4042, 30),
    ('Argos_Errante',   4042, 30),
    ('Elfo_Negro_Abj',  4042, 30),
    ('Cav._Elfo_Negro', 4042, 13),
    ('CH_Troll_Ghoul',  4042,  1),
    ('Troll_Ghoul',     4042,  1)
ON CONFLICT (mob, item) DO UPDATE SET chance = EXCLUDED.chance, updated_at = now();

UPDATE drop_rule_meta SET version = version + 1 WHERE id = TRUE;
