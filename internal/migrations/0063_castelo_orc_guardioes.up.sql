-- 0063_castelo_orc_guardioes — o saque novo dos dois primeiros guardiões do
-- Castelo Orc e o do Mago Orc.
--
-- Pedido da equipe em 14/09/2026. O Sentinela Orc (Portão Sul) e o Capitão Orc
-- (o portão seguinte) deixam de carregar a chave de portão no template e passam
-- a soltar, cada um:
--
--   Âmago de Cav. s/ Sela N 2396, pacote de 10     5%
--   Âmago de Cav. s/ Sela B 2401, pacote de 10     5%   (N ou B: ~10% juntos)
--   Moeda de Prata (5Mi) 4027                     10%
--   Pergaminho da Água (N) LV1 3173, pacote de 3   5%
--
-- A Mesa não guarda quantidade: o tamanho do pacote é do tmServer
-- (handler/castelo_orc.go, casteloOrcGuardianPacks). O saque que os dois já
-- davam continua como está.
--
-- O Mago Orc (COrc_Mago) é tropa nova, no lugar de metade dos Meio Orcs, e herda
-- o saque do Meio Orc como ele estiver na mesa na hora do deploy — inclusive o
-- que o painel já tiver mudado.
--
-- ON CONFLICT DO NOTHING, como na 0053: uma linha gravada pelo painel antes do
-- deploy vale mais que a proposta.

INSERT INTO drop_rule (mob, item, chance) VALUES
    ('COrc_Sentinela', 2396,  500), ('COrc_Capitao', 2396,  500),
    ('COrc_Sentinela', 2401,  500), ('COrc_Capitao', 2401,  500),
    ('COrc_Sentinela', 4027, 1000), ('COrc_Capitao', 4027, 1000),
    ('COrc_Sentinela', 3173,  500), ('COrc_Capitao', 3173,  500)
ON CONFLICT (mob, item) DO NOTHING;

INSERT INTO drop_rule (mob, item, chance)
SELECT 'COrc_Mago', item, chance FROM drop_rule WHERE mob = 'COrc_MeioOrc'
ON CONFLICT (mob, item) DO NOTHING;

-- O tmServer relê a mesa quando a versão muda.
UPDATE drop_rule_meta SET version = version + 1 WHERE id = TRUE;
