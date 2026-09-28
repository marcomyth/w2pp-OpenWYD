-- 0178_golem_lava_drop_tres_vezes_menos — o Golem de Pedra da sala de lava solta
-- três vezes menos (pedido do Marco em 28/09/2026).
--
-- É o golem que o jogador vê como "Golem de Pedra" na sala de lava (x 964-985,
-- y 3984-4005): o template Golem_Lava, cópia do Golem_de_Pedra criada pela 0148
-- para o saque valer só na sala. É ele que solta Diamante; o Golem de Pedra do
-- resto do 2º andar só tem os Âmagos Sem Sela da 0091 e não muda.
--
-- Toda chance acima de zero da 0148 cai para um terço, arredondando para baixo
-- ("ao menos 3×"). Os itens do template já estão a 0 desde a 0142, então esta é a
-- mesa inteira dele. O Anf Ninja da mesma sala (Anf_Ninja_Lava) não muda: o
-- pedido foi o golem.
--
-- A Mesa sorteia rand() % 10000 sobre o rand() de 15 bits do MSVC, e abaixo de
-- 27,68% paga 4/3,2768 do escrito; o desvio é o mesmo antes e depois, então a
-- razão real continua sendo a do número escrito.
INSERT INTO drop_rule (mob, item, chance) VALUES
    ('Golem_Lava', 2392, 16),  -- Âmago de Lobo            (era 50)
    ('Golem_Lava', 2394, 16),  -- Âmago de Urso            (era 50)
    ('Golem_Lava', 2395, 13),  -- Âmago de Dente de Sabre  (era 40)
    ('Golem_Lava', 2441, 10),  -- Diamante                 (era 30)
    ('Golem_Lava', 4019, 16),  -- Classe D                 (era 50)
    ('Golem_Lava', 4018, 13),  -- Classe C                 (era 40)
    ('Golem_Lava', 4026, 10),  -- Moeda de Prata(1Mi)      (era 30)
    ('Golem_Lava', 2396,  6),  -- Âmago de Cav s/Sela N    (era 20)
    ('Golem_Lava', 2401,  5)   -- Âmago de Ca s/Sela B     (era 15)
ON CONFLICT (mob, item) DO UPDATE SET chance = EXCLUDED.chance, updated_at = now();

UPDATE drop_rule_meta SET version = version + 1 WHERE id = TRUE;
