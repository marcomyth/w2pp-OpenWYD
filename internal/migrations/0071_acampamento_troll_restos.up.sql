-- 0071_acampamento_troll_restos — os Restos de Oriharucon e de Lactolerium no
-- saque do Acampamento Troll.
--
-- Pedido do Marco em 16/09/2026, depois do primeiro teste da quest. Uma unidade por
-- drop, sem pacote. Chance em centésimos de por cento:
--
--                                   Resto de Ori 419   Resto de Lac 420
--   Tropa (Insano, Caçador)         5%                 2%
--   Seguidor (Mago)                 5%                 2%
--   Guardião (Caos)                 15%                7,5%
--   Boss (Enigma)                   50%                30%
--
-- ON CONFLICT DO NOTHING: nenhum destes monstros soltava Resto antes, então uma
-- linha que já exista foi gravada pelo painel e vale mais que a proposta.

INSERT INTO drop_rule (mob, item, chance) VALUES
    ('ATroll_Insano',  419,  500), ('ATroll_Insano',  420,  200),
    ('ATroll_Cacador', 419,  500), ('ATroll_Cacador', 420,  200),
    ('ATroll_Mago',    419,  500), ('ATroll_Mago',    420,  200),
    ('ATroll_Caos',    419, 1500), ('ATroll_Caos',    420,  750),
    ('ATroll_Enigma',  419, 5000), ('ATroll_Enigma',  420, 3000)
ON CONFLICT (mob, item) DO NOTHING;

-- O tmServer relê a mesa quando a versão muda.
UPDATE drop_rule_meta SET version = version + 1 WHERE id = TRUE;
