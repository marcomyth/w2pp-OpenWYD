-- 0139_lorde_orc_e_frenzy — a Pedra do Lorde Orc sai do jogo, e o FrenzyDemonLord
-- vira chefe de 4 em 4 horas com o saque a 1%.
--
-- Pedido do Marco em 25/09/2026, depois da simulação da HT Xorimpas com as
-- chances da 0136.
--
-- A Pedra do Lorde Orc (1752) sai dos três templates de Lorde Orc que a carregam.
-- Só o Orc_L_Trooper nasce (Azran, blocos 1301 e 1303), e com a 0136 a HT tirava
-- uma pedra a cada ~42 minutos. O Lord_Trooper@@ (Cubo N, mapa de evento) e o
-- OrcLordTrooper (sem bloco) entram para a pedra não voltar num evento em que um
-- GM os crie. A família baixa das Pedras Arch (1744-1747) continua saindo das
-- pedras do Esqueleto, do Dragão Lich e do Demonlord.
--
-- O FrenzyDemonLord: a Pedra do Rei Demonlord já está a 81 (0136) e o Fragmento
-- de Alma desce de 10% (0090) para o mesmo 81 = 0,99% por morte. O renascimento
-- de 4 horas é código (handler/submundo.go) mais o bloco 3134 a MinuteGenerate
-- -1 no NPCGener. O segundo bloco dele (3135), no mesmo ponto, voltava em 15 s
-- porque com Exp 2.000 não é chefe sozinho; a 0090 o chamava de "de evento", e
-- aqui ele passa a ser: desligado pela chave da 0048, como os Agmo da 0109.
INSERT INTO drop_rule (mob, item, chance) VALUES
    ('Orc_L_Trooper',   1752,  0),        -- Pedra do Lorde Orc
    ('Lord_Trooper@@',  1752,  0),        -- Pedra do Lorde Orc
    ('OrcLordTrooper',  1752,  0),        -- Pedra do Lorde Orc
    ('FrenzyDemonLord', 3224, 81)         -- Fragmento de Alma
ON CONFLICT (mob, item) DO UPDATE SET chance = EXCLUDED.chance, updated_at = now();

UPDATE drop_rule_meta SET version = version + 1 WHERE id = TRUE;

INSERT INTO npc_generator_off (generator_index, turned_off_by) VALUES
    (3135, 'migração 0139')
ON CONFLICT (generator_index) DO NOTHING;

UPDATE npc_generator_off_meta SET version = version + 1 WHERE id = TRUE;
