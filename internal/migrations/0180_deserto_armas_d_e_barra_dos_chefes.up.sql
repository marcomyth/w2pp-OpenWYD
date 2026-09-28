-- 0180_deserto_armas_d_e_barra_dos_chefes — pedido do Marco em 28/09/2026.
--
-- 1) As Armas D do Deserto com o dobro da chance. A Mantícora, o Tauron
--    Adamantita, o Taron Assassino e o Verme soltam Armas D pelo template, a 0,05%
--    por vaga (vagas 24-37, g_pDropRate 2000). Aqui cada uma passa para a Mesa com
--    o dobro do que o template pagava a quem mata sem bônus de drop, e a Mesa as
--    entrega com o add do spot dos Ciclopes (handler/deserto_armas.go): dano 45-72
--    ou magia 20-32.
--      - Taron Assassino: dez armas, cada uma em UMA vaga (0,05%) -> 8 (0,098% real);
--      - Adamantita, Mantícora e Verme: cada arma em QUATRO vagas (0,2%) -> 33
--        (0,40% real).
--    A Mesa sorteia rand() % 10000 sobre o rand() de 15 bits do MSVC e, abaixo de
--    27,68%, paga 4/3,2768 do escrito: 8 sai 0,098% e 33 sai 0,40%.
--    O sorteio da Mesa não olha o bônus de drop de quem mata; o do template olhava.
--    Para quem joga com +100% de bônus, a chance fica igual à de antes.
--    O Escudo de Runas do Verme continua no template: é escudo, não ganha add.
--
-- 2) Barra de Prata (100Mi), item 4010, fixa em toda morte dos quatro chefes: o
--    Boss Mantícora, o Boss Hidra Dourada, o Boss Dragão Lich e a Frenzy Hidra.
--    Vem além do prêmio que cada um já sorteia. 10000 é 100%: rand() % 10000 é
--    sempre menor que 10000.
INSERT INTO drop_rule (mob, item, chance) VALUES
    ('Taron_Assassino', 869, 8),   -- Gram
    ('Taron_Assassino', 910, 8),   -- Luna
    ('Taron_Assassino', 809, 8),   -- Martelo Dragão
    ('Taron_Assassino', 935, 8),   -- Martelo Psíquico
    ('Taron_Assassino', 854, 8),   -- Gungnir
    ('Taron_Assassino', 902, 8),   -- Cajado de Âmbar
    ('Taron_Assassino', 899, 8),   -- Olho do Carbunkle
    ('Taron_Assassino', 824, 8),   -- Arco Élfico
    ('Taron_Assassino', 839, 8),   -- Presas de Behemoth
    ('Taron_Assassino', 884, 8),   -- Lança Relâmpago
    ('Adamant_Tauron',  936, 33),  -- Mjolnir
    ('Adamant_Tauron',  855, 33),  -- Lança do Triunfo
    ('Adamant_Tauron',  902, 33),  -- Cajado de Âmbar
    ('Manticora',       870, 33),  -- Espada Vorpal
    ('Manticora',       911, 33),  -- Solaris
    ('Manticora',       810, 33),  -- Martelo Assassino
    ('Verme_',          885, 33),  -- Cruz Sagrada
    ('Boss_Manticora',     4010, 10000),  -- Barra de Prata (100Mi)
    ('Boss_Hidra_Dourada', 4010, 10000),
    ('Boss_Dragao_Lich',   4010, 10000),
    ('Frenzy_Hidra',       4010, 10000)
ON CONFLICT (mob, item) DO UPDATE SET chance = EXCLUDED.chance, updated_at = now();

UPDATE drop_rule_meta SET version = version + 1 WHERE id = TRUE;

-- 3) O Ciclope Tirano com vida e dano x3 (1,8 milhão e 1.818). O arquivo do
--    template já traz os números novos; se o painel tiver uma ficha dele, é ela que
--    vale no jogo, e aí a ficha sobe x3 também. Sem ficha, nada acontece aqui.
UPDATE mob_template_stat
   SET max_hp = max_hp * 3, hp = hp * 3, damage = damage * 3, updated_at = now()
 WHERE template_name = 'Ciclope_Tirano';
