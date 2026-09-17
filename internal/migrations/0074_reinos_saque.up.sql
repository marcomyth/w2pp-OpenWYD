-- 0074_reinos_saque — o saque da invasão dos Reinos, e as Almas que abrem o Arch
-- só nos Reis.
--
-- Desenho fechado em 17/09/2026 (Atlas de Quests, área "Reinos e Reis";
-- docs/reinos.md). Chance em centésimos de por cento. O tamanho do pacote não
-- cabe na mesa e fica no tmServer (reinosPacotes, handler/reinos.go). O âmago
-- segue a cor do reino: Andaluz B e Fenrir em Hekalotia (azul), Andaluz N e
-- Fenrir das Sombras em Akelonia (vermelho).
--
--                                  Tropa      Elite      Escolta     Rei
--   Alma do Unicórnio 1740         —          —          —           5% (Harabard)
--   Alma da Fênix 1741             —          —          —           5% (Glantuar)
--   Fragmento de Alma 3224         —          —          2%          —
--   Âmago de Andaluz B/N 2405/2400 2% ×5      8% ×10     —           —
--   Âmago de Fenrir 2406/2408      —          —          12% ×5      50% ×10
--   Moeda de Prata (1Mi) 4026      3%         —          —           —
--   Moeda de Prata (5Mi) 4027      —          5%         15%         100% ×10
--   Poeira de Oriharucon 412       8%         15%        25%         100% ×30
--   Poeira de Lactolerium 413      3%         8%         25%         100% ×15
--   Classe C 4018                  6% ×5      —          —           —
--   Classe B 4017                  —          8% ×10     —           —
--   Classe A 4016                  —          —          10% ×10     100% ×20
--
-- Tropa: Guarda do Rei, Combatente, Lanceiro, Virago e Bruxa. Elite: Cavaleiro
-- Real, Averest e Feiticeira. Escolta: Escolta Real. Os de Akelonia têm o mesmo
-- nome com "_" no fim.
--
-- As Almas e a Pedra da Imortalidade saem de todo monstro (regra "*" a 0%), e a
-- regra de cada Rei, por ser de um monstro nomeado, vale por cima dela. Hoje elas
-- caíam de Arvak, Balrog, Cav. Lugefer, Valquíria, Bruxa Negra, Imp do Inferno,
-- Lich Batama, Sombra Negra e Evolucao; sem isto o Reino não segura o ritmo do
-- Arch.
--
-- Uma regra nomeada vale por cima do "*": se o painel já tiver posto uma Alma ou a
-- Pedra em algum monstro, ela continuaria caindo. Essas regras saem aqui, e o
-- down não as devolve (não há como saber quais eram).
--
-- ON CONFLICT DO UPDATE: o pedido é justamente fixar estes números, por cima do
-- que o painel tiver gravado.

DELETE FROM drop_rule
WHERE item IN (1740, 1741, 1742) AND mob NOT IN ('*', 'Rei_Harabard', 'Rei_Glantuar');

INSERT INTO drop_rule (mob, item, chance) VALUES
    ('*', 1740, 0), ('*', 1741, 0), ('*', 1742, 0),

    ('Rei_Harabard', 1740,   500), ('Rei_Harabard', 2406,  5000), ('Rei_Harabard', 4027, 10000),
    ('Rei_Harabard',  412, 10000), ('Rei_Harabard',  413, 10000), ('Rei_Harabard', 4016, 10000),
    ('Rei_Glantuar', 1741,   500), ('Rei_Glantuar', 2408,  5000), ('Rei_Glantuar', 4027, 10000),
    ('Rei_Glantuar',  412, 10000), ('Rei_Glantuar',  413, 10000), ('Rei_Glantuar', 4016, 10000),

    ('Escolta_Real',  3224, 200), ('Escolta_Real',  2406, 1200), ('Escolta_Real',  4027, 1500),
    ('Escolta_Real',   412, 2500), ('Escolta_Real',   413, 2500), ('Escolta_Real',  4016, 1000),
    ('Escolta_Real_', 3224, 200), ('Escolta_Real_', 2408, 1200), ('Escolta_Real_', 4027, 1500),
    ('Escolta_Real_',  412, 2500), ('Escolta_Real_',  413, 2500), ('Escolta_Real_', 4016, 1000),

    ('Cav._Real',   2405, 800), ('Cav._Real',   4027, 500), ('Cav._Real',   412, 1500), ('Cav._Real',   413, 800), ('Cav._Real',   4017, 800),
    ('Cav._Real_',  2400, 800), ('Cav._Real_',  4027, 500), ('Cav._Real_',  412, 1500), ('Cav._Real_',  413, 800), ('Cav._Real_',  4017, 800),
    ('Averest',     2405, 800), ('Averest',     4027, 500), ('Averest',     412, 1500), ('Averest',     413, 800), ('Averest',     4017, 800),
    ('Averest_',    2400, 800), ('Averest_',    4027, 500), ('Averest_',    412, 1500), ('Averest_',    413, 800), ('Averest_',    4017, 800),
    ('Feiticeira',  2405, 800), ('Feiticeira',  4027, 500), ('Feiticeira',  412, 1500), ('Feiticeira',  413, 800), ('Feiticeira',  4017, 800),
    ('Feiticeira_', 2400, 800), ('Feiticeira_', 4027, 500), ('Feiticeira_', 412, 1500), ('Feiticeira_', 413, 800), ('Feiticeira_', 4017, 800),

    ('Guarda_do_Rei',  2405, 200), ('Guarda_do_Rei',  4026, 300), ('Guarda_do_Rei',  412, 800), ('Guarda_do_Rei',  413, 300), ('Guarda_do_Rei',  4018, 600),
    ('Guarda_do_Rei_', 2400, 200), ('Guarda_do_Rei_', 4026, 300), ('Guarda_do_Rei_', 412, 800), ('Guarda_do_Rei_', 413, 300), ('Guarda_do_Rei_', 4018, 600),
    ('Combatente',     2405, 200), ('Combatente',     4026, 300), ('Combatente',     412, 800), ('Combatente',     413, 300), ('Combatente',     4018, 600),
    ('Combatente_',    2400, 200), ('Combatente_',    4026, 300), ('Combatente_',    412, 800), ('Combatente_',    413, 300), ('Combatente_',    4018, 600),
    ('Lanceiro',       2405, 200), ('Lanceiro',       4026, 300), ('Lanceiro',       412, 800), ('Lanceiro',       413, 300), ('Lanceiro',       4018, 600),
    ('Lanceiro_',      2400, 200), ('Lanceiro_',      4026, 300), ('Lanceiro_',      412, 800), ('Lanceiro_',      413, 300), ('Lanceiro_',      4018, 600),
    ('Virago',         2405, 200), ('Virago',         4026, 300), ('Virago',         412, 800), ('Virago',         413, 300), ('Virago',         4018, 600),
    ('Virago_',        2400, 200), ('Virago_',        4026, 300), ('Virago_',        412, 800), ('Virago_',        413, 300), ('Virago_',        4018, 600),
    ('Bruxa',          2405, 200), ('Bruxa',          4026, 300), ('Bruxa',          412, 800), ('Bruxa',          413, 300), ('Bruxa',          4018, 600),
    ('Bruxa_',         2400, 200), ('Bruxa_',         4026, 300), ('Bruxa_',         412, 800), ('Bruxa_',         413, 300), ('Bruxa_',         4018, 600)
ON CONFLICT (mob, item) DO UPDATE SET chance = EXCLUDED.chance, updated_at = now();

-- O tmServer relê a mesa quando a versão muda.
UPDATE drop_rule_meta SET version = version + 1 WHERE id = TRUE;
