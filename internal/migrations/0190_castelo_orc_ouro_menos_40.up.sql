-- 0190_castelo_orc_ouro_menos_40 — o ouro do Castelo Orc cai 40% (pedido do
-- Marco em 01/10/2026).
--
-- O ouro da quest são as Moedas de Prata da Mesa de Drops: o Coin do template
-- não é lido, então nenhum orc dá ouro direto. A 0053 pôs a Moeda de Prata (1Mi)
-- a 16% na tropa e nos guardiões para uma meta de 10 moedas por entrada, contando
-- 63 abates. Desde 16/09 a tropa renasce a cada 30 s (o Grão-Lorde só vem no 100º
-- abate), e um grupo que fica os 15 minutos derruba mais de mil orcs: a corrida
-- de 6 contas de 01/10 soltou ~170 moedas em 11 minutos e meio, pelo log
-- "drop table hit" de produção.
--
-- O pedido foi cortar 40% do ouro, não voltar à meta de 10:
--
--   Moeda de Prata (1Mi) 4026, tropa e guardiões   16% -> 9,6%
--   Moeda de Prata (5Mi) 4027, Sentinela e Capitão 10% -> 6%
--
-- A Mesa sorteia rand() % 10000 sobre o rand() de 15 bits do MSVC, e abaixo de
-- 27,68% paga 4/3,2768 do escrito; o desvio é o mesmo antes e depois, então o
-- corte real é o do número escrito.
--
-- Valor absoluto, e não chance * 0,6: uma conta sobre a própria coluna compõe se
-- a migração rodar de novo (upsert_absoluto_test.go). A linha que o painel tiver
-- mudado é sobrescrita.
INSERT INTO drop_rule (mob, item, chance) VALUES
    ('COrc_Cavaleiro', 4026, 960), ('COrc_Arqueiro', 4026, 960),
    ('COrc_MeioOrc',   4026, 960), ('COrc_Mago',     4026, 960),
    ('COrc_Sentinela', 4026, 960), ('COrc_Capitao',  4026, 960), ('COrc_Chefe', 4026, 960),
    ('COrc_Sentinela', 4027, 600), ('COrc_Capitao',  4027, 600)
ON CONFLICT (mob, item) DO UPDATE SET chance = EXCLUDED.chance, updated_at = now();

-- O tmServer relê a mesa quando a versão muda.
UPDATE drop_rule_meta SET version = version + 1 WHERE id = TRUE;
