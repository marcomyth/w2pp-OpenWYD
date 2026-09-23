-- 0110_gelo_saque — a mesa de saque do Gelo.
--
-- O Gelo não tem caixa própria no Regions.txt: a tropa nasce toda na caixa
-- Karden (3391-4027 × 2649-3255), em volta de Nippleheim e da Vila Amald. São 317
-- monstros comuns de 20 templates, do nível 300 ao 395, e cinco chefes (Verid,
-- Verid_, as duas Sombras Negras e a Serva de Odin). Antes disto um comum soltava
-- algo útil a cada ~451 abates. Uma regra da Mesa vale pelo nome do template, e
-- só um deles nasce também fora daqui: o Verid, no Coliseu (bloco 104). As linhas
-- dele são todas a 0%, então o que vaza para lá é só a retirada, nunca saque
-- (TestGeloMigracaoFicaNoGelo cobra isso).
--
-- As chances estão em centésimos de por cento, como a Mesa grava e o painel
-- mostra; abaixo de 27,68% o sorteio paga 4/3,2768 do escrito
-- (internal/droprule/vies_test.go): 100 sai 1,22%, 33 sai 0,40%.
--
-- O que SAI do Gelo, decidido pela equipe em 23/09 — linhas a 0% em todo template
-- da área que o soltava, chefes inclusive:
--   Fenrir e Andaluz, âmago e ovo, "por enquanto", como no Deserto (0109);
--   Unicórnio, Pégasus, Unisus e o Grifo Sangrento, âmago e ovo;
--   as montarias N (Fantasma N no Lobo Polar, ovo Equipado N no Guerreiro Amon,
--   âmago Equipado N no Verid): o Gelo é a casa das brancas, e o Deserto das N;
--   a Pedra da Luz (3140), de oito templates;
--   as armas e os sets Arch: todo item com EF_MOBTYPE 1 (ARCH, Basedef.h:239) que
--   a área soltava — Balmung, Caliburn, Skytalos, Khyrius, Gleipnir, Hermai,
--   Eirenus, Neorion, Thrasytes, Basileus, Hophlon e os sets Flamejante, Guardiã,
--   Destruição e Rake. As armas de EF_MOBTYPE 2 (MORTAL: Éden, Demolidor
--   Celestial, Dianus…) não são Arch e ficam.
--
-- O que ENTRA, decidido pela equipe em 23/09:
--   todo comum: Poeira de Oriharucon 1%, de Lactolerium 0,5%, Resto de
--   Oriharucon 1%, de Lactolerium 0,5% e Moeda de Prata (1Mi) 0,3%;
--   Vila Amald: o Set E, as 20 peças de nível 5 que não são Arch (Mortal,
--   Templário, Corvo e Legionário), a 0,15% cada; o LE da 0080 fica;
--   Homem e Mulher Kalintz: as dez armas E só de Mortal (EF_MOBTYPE 2: Éden,
--   Demolidor Celestial, Dianus, Força Eterna, Vingadora, Asa Draconiana,
--   Karikas, Arco Guardião, Foice Platinada, Cajado Caótico) e o Escudo do
--   Guardião, a 0,1% cada — toda arma E do jogo que não é Mortal é Arch;
--   Ent Ancião, Lobo Polar, Grande Lobo e Urso Polar: os Sets A de nível 4 (40
--   peças: Anão, Embutido, Conjurador, Mytril, Aeon, Elemental, Natureza, Teia) a
--   0,03% cada, e o topo das armas D (Martelo Assassino, Arco Divino, Garra
--   Draconiana, Lança do Triunfo, Espada Vorpal, Cruz Sagrada, Fúria Divina,
--   Solaris, Mjolnir) e os escudos D (Aegis, Escudo de Runas) a 0,05% cada;
--   Símio Ancião, o macaco fraco: âmagos B até o Cavalo Leve (Sem Sela, Fantasma,
--   Leve) a 0,33% e os ovos a 0,05%;
--   Soldado, Capitão e Guerreiro Amon, os macacos fortes: âmago do Cavalo
--   Equipado B a 0,33% e o ovo a 0,05%.
--   Valquírias Rosen e Tina: os Sets Mytril e Teia LE; Guarda Beriel: os Sets
--   Embutido e Elemental LE; 0,16% por peça, como a Vila Amald na 0080 (o "*" a 0%
--   de lá tira o LE do mundo, e a linha nomeada passa por cima).
-- Troll de Gelo, Tita Berserker e Berserker Ref ficam só com a base.
--
-- Os chefes (Sombras Negras e Verid) perdem aqui o que saiu, e o saque novo deles
-- é código (handler/gelo.go): uma coisa por morte, Pacote de Cavalo Equipado B,
-- o ovo, Fragmento de Alma, Barra de 50Mi ou RCoin de 100, e a Alma a 1%.
INSERT INTO drop_rule (mob, item, chance) VALUES
    -- Campo de gelo
    ('Lobo_Polar',       1223,    0),  -- Manoplas Flamejantes (Arch)
    ('Lobo_Polar',       1358,    0),  -- Manoplas Guardiãs (Arch)
    ('Lobo_Polar',       1508,    0),  -- Manoplas da Derstruição (Arch)
    ('Lobo_Polar',       1658,    0),  -- Manoplas Rake (Arch)
    ('Lobo_Polar',       2307,    0),  -- Ovo de Cavalo Fantasm N (montaria N (o Gelo é das brancas))
    ('Lobo_Polar',       2397,    0),  -- Âmago de Cav Fantasm N (montaria N (o Gelo é das brancas))
    ('Lobo_Polar',       3140,    0),  -- Pedra da Luz (Pedra da Luz)
    ('Lobo_Polar',        412,  100),  -- Poeira de Oriharucon
    ('Lobo_Polar',        413,   50),  -- Poeira de Lactolerium
    ('Lobo_Polar',        419,  100),  -- Resto de Oriharucon
    ('Lobo_Polar',        420,   50),  -- Resto de Lactolerium
    ('Lobo_Polar',        810,    5),  -- Martelo Assassino
    ('Lobo_Polar',        825,    5),  -- Arco Divino
    ('Lobo_Polar',        840,    5),  -- Garra Draconiana
    ('Lobo_Polar',        855,    5),  -- Lança do Triunfo
    ('Lobo_Polar',        870,    5),  -- Espada Vorpal
    ('Lobo_Polar',        885,    5),  -- Cruz Sagrada
    ('Lobo_Polar',        900,    5),  -- Fúria Divina
    ('Lobo_Polar',        911,    5),  -- Solaris
    ('Lobo_Polar',        936,    5),  -- Mjolnir
    ('Lobo_Polar',       1193,    3),  -- Elmo Anão(A)
    ('Lobo_Polar',       1196,    3),  -- Armadura Anã(A)
    ('Lobo_Polar',       1199,    3),  -- Calça Anã(A)
    ('Lobo_Polar',       1202,    3),  -- Manopla Anã(A)
    ('Lobo_Polar',       1205,    3),  -- Botas Anã(A)
    ('Lobo_Polar',       1208,    3),  -- Elmo Embutido(A)
    ('Lobo_Polar',       1211,    3),  -- Armadura Embutida(A)
    ('Lobo_Polar',       1214,    3),  -- Calça Embutida(A)
    ('Lobo_Polar',       1217,    3),  -- Manoplas Embutidas(A)
    ('Lobo_Polar',       1220,    3),  -- Botas Embutidas(A)
    ('Lobo_Polar',       1328,    3),  -- Chapéu Conjurador(A)
    ('Lobo_Polar',       1331,    3),  -- Túnica Conjuradora(A)
    ('Lobo_Polar',       1334,    3),  -- Calça Conjuradora(A)
    ('Lobo_Polar',       1337,    3),  -- Luvas Conjuradoras(A)
    ('Lobo_Polar',       1340,    3),  -- Botas Conjuradoras(A)
    ('Lobo_Polar',       1343,    3),  -- Chapéu de Mytril(A)
    ('Lobo_Polar',       1346,    3),  -- Túnica de Mytril(A)
    ('Lobo_Polar',       1349,    3),  -- Calça de Mytril(A)
    ('Lobo_Polar',       1352,    3),  -- Luvas de Mytril(A)
    ('Lobo_Polar',       1355,    3),  -- Botas de Mytril(A)
    ('Lobo_Polar',       1478,    3),  -- Elmo Aeon(A)
    ('Lobo_Polar',       1481,    3),  -- Armadura Aeon(A)
    ('Lobo_Polar',       1484,    3),  -- Calça Aeon(A)
    ('Lobo_Polar',       1487,    3),  -- Manoplas Aeon(A)
    ('Lobo_Polar',       1490,    3),  -- Botas Aeon(A)
    ('Lobo_Polar',       1493,    3),  -- Elmo Elemental(A)
    ('Lobo_Polar',       1496,    3),  -- Armadura Elemental(A)
    ('Lobo_Polar',       1499,    3),  -- Calça Elemental(A)
    ('Lobo_Polar',       1502,    3),  -- Manoplas Elementais(A)
    ('Lobo_Polar',       1505,    3),  -- Botas Elementais(A)
    ('Lobo_Polar',       1628,    3),  -- Chapéu da Natureza(A)
    ('Lobo_Polar',       1631,    3),  -- Peitoral da Natureza(A)
    ('Lobo_Polar',       1634,    3),  -- Calça da Natureza(A)
    ('Lobo_Polar',       1637,    3),  -- Luvas da Natureza(A)
    ('Lobo_Polar',       1640,    3),  -- Botas da Natureza(A)
    ('Lobo_Polar',       1643,    3),  -- Chapéu de Teia(A)
    ('Lobo_Polar',       1646,    3),  -- Peitoral de Teia(A)
    ('Lobo_Polar',       1649,    3),  -- Calça de Teia(A)
    ('Lobo_Polar',       1652,    3),  -- Braçadeira de Teia(A)
    ('Lobo_Polar',       1655,    3),  -- Botas de Teia(A)
    ('Lobo_Polar',       1709,    5),  -- Aegis
    ('Lobo_Polar',       1710,    5),  -- Escudo de Runas
    ('Lobo_Polar',       4026,   30),  -- Moeda de Prata(1Mi)
    ('Urso_Polar',       1223,    0),  -- Manoplas Flamejantes (Arch)
    ('Urso_Polar',       1224,    0),  -- Botas Flamejantes (Arch)
    ('Urso_Polar',       1358,    0),  -- Manoplas Guardiãs (Arch)
    ('Urso_Polar',       1359,    0),  -- Botas Guardiãs (Arch)
    ('Urso_Polar',       1508,    0),  -- Manoplas da Derstruição (Arch)
    ('Urso_Polar',       1509,    0),  -- Botas da Destruição (Arch)
    ('Urso_Polar',       1658,    0),  -- Manoplas Rake (Arch)
    ('Urso_Polar',       1659,    0),  -- Botas Rake (Arch)
    ('Urso_Polar',       3140,    0),  -- Pedra da Luz (Pedra da Luz)
    ('Urso_Polar',        412,  100),  -- Poeira de Oriharucon
    ('Urso_Polar',        413,   50),  -- Poeira de Lactolerium
    ('Urso_Polar',        419,  100),  -- Resto de Oriharucon
    ('Urso_Polar',        420,   50),  -- Resto de Lactolerium
    ('Urso_Polar',        810,    5),  -- Martelo Assassino
    ('Urso_Polar',        825,    5),  -- Arco Divino
    ('Urso_Polar',        840,    5),  -- Garra Draconiana
    ('Urso_Polar',        855,    5),  -- Lança do Triunfo
    ('Urso_Polar',        870,    5),  -- Espada Vorpal
    ('Urso_Polar',        885,    5),  -- Cruz Sagrada
    ('Urso_Polar',        900,    5),  -- Fúria Divina
    ('Urso_Polar',        911,    5),  -- Solaris
    ('Urso_Polar',        936,    5),  -- Mjolnir
    ('Urso_Polar',       1193,    3),  -- Elmo Anão(A)
    ('Urso_Polar',       1196,    3),  -- Armadura Anã(A)
    ('Urso_Polar',       1199,    3),  -- Calça Anã(A)
    ('Urso_Polar',       1202,    3),  -- Manopla Anã(A)
    ('Urso_Polar',       1205,    3),  -- Botas Anã(A)
    ('Urso_Polar',       1208,    3),  -- Elmo Embutido(A)
    ('Urso_Polar',       1211,    3),  -- Armadura Embutida(A)
    ('Urso_Polar',       1214,    3),  -- Calça Embutida(A)
    ('Urso_Polar',       1217,    3),  -- Manoplas Embutidas(A)
    ('Urso_Polar',       1220,    3),  -- Botas Embutidas(A)
    ('Urso_Polar',       1328,    3),  -- Chapéu Conjurador(A)
    ('Urso_Polar',       1331,    3),  -- Túnica Conjuradora(A)
    ('Urso_Polar',       1334,    3),  -- Calça Conjuradora(A)
    ('Urso_Polar',       1337,    3),  -- Luvas Conjuradoras(A)
    ('Urso_Polar',       1340,    3),  -- Botas Conjuradoras(A)
    ('Urso_Polar',       1343,    3),  -- Chapéu de Mytril(A)
    ('Urso_Polar',       1346,    3),  -- Túnica de Mytril(A)
    ('Urso_Polar',       1349,    3),  -- Calça de Mytril(A)
    ('Urso_Polar',       1352,    3),  -- Luvas de Mytril(A)
    ('Urso_Polar',       1355,    3),  -- Botas de Mytril(A)
    ('Urso_Polar',       1478,    3),  -- Elmo Aeon(A)
    ('Urso_Polar',       1481,    3),  -- Armadura Aeon(A)
    ('Urso_Polar',       1484,    3),  -- Calça Aeon(A)
    ('Urso_Polar',       1487,    3),  -- Manoplas Aeon(A)
    ('Urso_Polar',       1490,    3),  -- Botas Aeon(A)
    ('Urso_Polar',       1493,    3),  -- Elmo Elemental(A)
    ('Urso_Polar',       1496,    3),  -- Armadura Elemental(A)
    ('Urso_Polar',       1499,    3),  -- Calça Elemental(A)
    ('Urso_Polar',       1502,    3),  -- Manoplas Elementais(A)
    ('Urso_Polar',       1505,    3),  -- Botas Elementais(A)
    ('Urso_Polar',       1628,    3),  -- Chapéu da Natureza(A)
    ('Urso_Polar',       1631,    3),  -- Peitoral da Natureza(A)
    ('Urso_Polar',       1634,    3),  -- Calça da Natureza(A)
    ('Urso_Polar',       1637,    3),  -- Luvas da Natureza(A)
    ('Urso_Polar',       1640,    3),  -- Botas da Natureza(A)
    ('Urso_Polar',       1643,    3),  -- Chapéu de Teia(A)
    ('Urso_Polar',       1646,    3),  -- Peitoral de Teia(A)
    ('Urso_Polar',       1649,    3),  -- Calça de Teia(A)
    ('Urso_Polar',       1652,    3),  -- Braçadeira de Teia(A)
    ('Urso_Polar',       1655,    3),  -- Botas de Teia(A)
    ('Urso_Polar',       1709,    5),  -- Aegis
    ('Urso_Polar',       1710,    5),  -- Escudo de Runas
    ('Urso_Polar',       4026,   30),  -- Moeda de Prata(1Mi)
    ('Troll_de_Gelo',    1508,    0),  -- Manoplas da Derstruição (Arch)
    ('Troll_de_Gelo',    1658,    0),  -- Manoplas Rake (Arch)
    ('Troll_de_Gelo',    3140,    0),  -- Pedra da Luz (Pedra da Luz)
    ('Troll_de_Gelo',     412,  100),  -- Poeira de Oriharucon
    ('Troll_de_Gelo',     413,   50),  -- Poeira de Lactolerium
    ('Troll_de_Gelo',     419,  100),  -- Resto de Oriharucon
    ('Troll_de_Gelo',     420,   50),  -- Resto de Lactolerium
    ('Troll_de_Gelo',    4026,   30),  -- Moeda de Prata(1Mi)
    ('Ent_Anciao',       1509,    0),  -- Botas da Destruição (Arch)
    ('Ent_Anciao',       1659,    0),  -- Botas Rake (Arch)
    ('Ent_Anciao',       2322,    0),  -- Ovo de Pegasus (Fenrir e montaria alta)
    ('Ent_Anciao',        412,  100),  -- Poeira de Oriharucon
    ('Ent_Anciao',        413,   50),  -- Poeira de Lactolerium
    ('Ent_Anciao',        419,  100),  -- Resto de Oriharucon
    ('Ent_Anciao',        420,   50),  -- Resto de Lactolerium
    ('Ent_Anciao',        810,    5),  -- Martelo Assassino
    ('Ent_Anciao',        825,    5),  -- Arco Divino
    ('Ent_Anciao',        840,    5),  -- Garra Draconiana
    ('Ent_Anciao',        855,    5),  -- Lança do Triunfo
    ('Ent_Anciao',        870,    5),  -- Espada Vorpal
    ('Ent_Anciao',        885,    5),  -- Cruz Sagrada
    ('Ent_Anciao',        900,    5),  -- Fúria Divina
    ('Ent_Anciao',        911,    5),  -- Solaris
    ('Ent_Anciao',        936,    5),  -- Mjolnir
    ('Ent_Anciao',       1193,    3),  -- Elmo Anão(A)
    ('Ent_Anciao',       1196,    3),  -- Armadura Anã(A)
    ('Ent_Anciao',       1199,    3),  -- Calça Anã(A)
    ('Ent_Anciao',       1202,    3),  -- Manopla Anã(A)
    ('Ent_Anciao',       1205,    3),  -- Botas Anã(A)
    ('Ent_Anciao',       1208,    3),  -- Elmo Embutido(A)
    ('Ent_Anciao',       1211,    3),  -- Armadura Embutida(A)
    ('Ent_Anciao',       1214,    3),  -- Calça Embutida(A)
    ('Ent_Anciao',       1217,    3),  -- Manoplas Embutidas(A)
    ('Ent_Anciao',       1220,    3),  -- Botas Embutidas(A)
    ('Ent_Anciao',       1328,    3),  -- Chapéu Conjurador(A)
    ('Ent_Anciao',       1331,    3),  -- Túnica Conjuradora(A)
    ('Ent_Anciao',       1334,    3),  -- Calça Conjuradora(A)
    ('Ent_Anciao',       1337,    3),  -- Luvas Conjuradoras(A)
    ('Ent_Anciao',       1340,    3),  -- Botas Conjuradoras(A)
    ('Ent_Anciao',       1343,    3),  -- Chapéu de Mytril(A)
    ('Ent_Anciao',       1346,    3),  -- Túnica de Mytril(A)
    ('Ent_Anciao',       1349,    3),  -- Calça de Mytril(A)
    ('Ent_Anciao',       1352,    3),  -- Luvas de Mytril(A)
    ('Ent_Anciao',       1355,    3),  -- Botas de Mytril(A)
    ('Ent_Anciao',       1478,    3),  -- Elmo Aeon(A)
    ('Ent_Anciao',       1481,    3),  -- Armadura Aeon(A)
    ('Ent_Anciao',       1484,    3),  -- Calça Aeon(A)
    ('Ent_Anciao',       1487,    3),  -- Manoplas Aeon(A)
    ('Ent_Anciao',       1490,    3),  -- Botas Aeon(A)
    ('Ent_Anciao',       1493,    3),  -- Elmo Elemental(A)
    ('Ent_Anciao',       1496,    3),  -- Armadura Elemental(A)
    ('Ent_Anciao',       1499,    3),  -- Calça Elemental(A)
    ('Ent_Anciao',       1502,    3),  -- Manoplas Elementais(A)
    ('Ent_Anciao',       1505,    3),  -- Botas Elementais(A)
    ('Ent_Anciao',       1628,    3),  -- Chapéu da Natureza(A)
    ('Ent_Anciao',       1631,    3),  -- Peitoral da Natureza(A)
    ('Ent_Anciao',       1634,    3),  -- Calça da Natureza(A)
    ('Ent_Anciao',       1637,    3),  -- Luvas da Natureza(A)
    ('Ent_Anciao',       1640,    3),  -- Botas da Natureza(A)
    ('Ent_Anciao',       1643,    3),  -- Chapéu de Teia(A)
    ('Ent_Anciao',       1646,    3),  -- Peitoral de Teia(A)
    ('Ent_Anciao',       1649,    3),  -- Calça de Teia(A)
    ('Ent_Anciao',       1652,    3),  -- Braçadeira de Teia(A)
    ('Ent_Anciao',       1655,    3),  -- Botas de Teia(A)
    ('Ent_Anciao',       1709,    5),  -- Aegis
    ('Ent_Anciao',       1710,    5),  -- Escudo de Runas
    ('Ent_Anciao',       4026,   30),  -- Moeda de Prata(1Mi)
    ('Grande_Lobo',      1221,    0),  -- Armadura Flamejante (Arch)
    ('Grande_Lobo',      1222,    0),  -- Calça Flamejante (Arch)
    ('Grande_Lobo',      1356,    0),  -- Túnica Guardiã (Arch)
    ('Grande_Lobo',      1357,    0),  -- Calça Guardiã (Arch)
    ('Grande_Lobo',      1506,    0),  -- Armadura da Destruição (Arch)
    ('Grande_Lobo',      1507,    0),  -- Calça da Destruição (Arch)
    ('Grande_Lobo',      1656,    0),  -- Armadura Rake (Arch)
    ('Grande_Lobo',      1657,    0),  -- Calça Rake (Arch)
    ('Grande_Lobo',      3140,    0),  -- Pedra da Luz (Pedra da Luz)
    ('Grande_Lobo',       412,  100),  -- Poeira de Oriharucon
    ('Grande_Lobo',       413,   50),  -- Poeira de Lactolerium
    ('Grande_Lobo',       419,  100),  -- Resto de Oriharucon
    ('Grande_Lobo',       420,   50),  -- Resto de Lactolerium
    ('Grande_Lobo',       810,    5),  -- Martelo Assassino
    ('Grande_Lobo',       825,    5),  -- Arco Divino
    ('Grande_Lobo',       840,    5),  -- Garra Draconiana
    ('Grande_Lobo',       855,    5),  -- Lança do Triunfo
    ('Grande_Lobo',       870,    5),  -- Espada Vorpal
    ('Grande_Lobo',       885,    5),  -- Cruz Sagrada
    ('Grande_Lobo',       900,    5),  -- Fúria Divina
    ('Grande_Lobo',       911,    5),  -- Solaris
    ('Grande_Lobo',       936,    5),  -- Mjolnir
    ('Grande_Lobo',      1193,    3),  -- Elmo Anão(A)
    ('Grande_Lobo',      1196,    3),  -- Armadura Anã(A)
    ('Grande_Lobo',      1199,    3),  -- Calça Anã(A)
    ('Grande_Lobo',      1202,    3),  -- Manopla Anã(A)
    ('Grande_Lobo',      1205,    3),  -- Botas Anã(A)
    ('Grande_Lobo',      1208,    3),  -- Elmo Embutido(A)
    ('Grande_Lobo',      1211,    3),  -- Armadura Embutida(A)
    ('Grande_Lobo',      1214,    3),  -- Calça Embutida(A)
    ('Grande_Lobo',      1217,    3),  -- Manoplas Embutidas(A)
    ('Grande_Lobo',      1220,    3),  -- Botas Embutidas(A)
    ('Grande_Lobo',      1328,    3),  -- Chapéu Conjurador(A)
    ('Grande_Lobo',      1331,    3),  -- Túnica Conjuradora(A)
    ('Grande_Lobo',      1334,    3),  -- Calça Conjuradora(A)
    ('Grande_Lobo',      1337,    3),  -- Luvas Conjuradoras(A)
    ('Grande_Lobo',      1340,    3),  -- Botas Conjuradoras(A)
    ('Grande_Lobo',      1343,    3),  -- Chapéu de Mytril(A)
    ('Grande_Lobo',      1346,    3),  -- Túnica de Mytril(A)
    ('Grande_Lobo',      1349,    3),  -- Calça de Mytril(A)
    ('Grande_Lobo',      1352,    3),  -- Luvas de Mytril(A)
    ('Grande_Lobo',      1355,    3),  -- Botas de Mytril(A)
    ('Grande_Lobo',      1478,    3),  -- Elmo Aeon(A)
    ('Grande_Lobo',      1481,    3),  -- Armadura Aeon(A)
    ('Grande_Lobo',      1484,    3),  -- Calça Aeon(A)
    ('Grande_Lobo',      1487,    3),  -- Manoplas Aeon(A)
    ('Grande_Lobo',      1490,    3),  -- Botas Aeon(A)
    ('Grande_Lobo',      1493,    3),  -- Elmo Elemental(A)
    ('Grande_Lobo',      1496,    3),  -- Armadura Elemental(A)
    ('Grande_Lobo',      1499,    3),  -- Calça Elemental(A)
    ('Grande_Lobo',      1502,    3),  -- Manoplas Elementais(A)
    ('Grande_Lobo',      1505,    3),  -- Botas Elementais(A)
    ('Grande_Lobo',      1628,    3),  -- Chapéu da Natureza(A)
    ('Grande_Lobo',      1631,    3),  -- Peitoral da Natureza(A)
    ('Grande_Lobo',      1634,    3),  -- Calça da Natureza(A)
    ('Grande_Lobo',      1637,    3),  -- Luvas da Natureza(A)
    ('Grande_Lobo',      1640,    3),  -- Botas da Natureza(A)
    ('Grande_Lobo',      1643,    3),  -- Chapéu de Teia(A)
    ('Grande_Lobo',      1646,    3),  -- Peitoral de Teia(A)
    ('Grande_Lobo',      1649,    3),  -- Calça de Teia(A)
    ('Grande_Lobo',      1652,    3),  -- Braçadeira de Teia(A)
    ('Grande_Lobo',      1655,    3),  -- Botas de Teia(A)
    ('Grande_Lobo',      1709,    5),  -- Aegis
    ('Grande_Lobo',      1710,    5),  -- Escudo de Runas
    ('Grande_Lobo',      4026,   30),  -- Moeda de Prata(1Mi)
    ('Simio_Anciao',     1224,    0),  -- Botas Flamejantes (Arch)
    ('Simio_Anciao',     1359,    0),  -- Botas Guardiãs (Arch)
    ('Simio_Anciao',      412,  100),  -- Poeira de Oriharucon
    ('Simio_Anciao',      413,   50),  -- Poeira de Lactolerium
    ('Simio_Anciao',      419,  100),  -- Resto de Oriharucon
    ('Simio_Anciao',      420,   50),  -- Resto de Lactolerium
    ('Simio_Anciao',     2311,    5),  -- Ovo de Cavalo s/Sela B
    ('Simio_Anciao',     2312,    5),  -- Ovo de Cavalo Fantasm B
    ('Simio_Anciao',     2313,    5),  -- Ovo de Cavalo Leve B
    ('Simio_Anciao',     2401,   33),  -- Âmago de Ca s/Sela B
    ('Simio_Anciao',     2402,   33),  -- Âmago de Cav Fantasm B
    ('Simio_Anciao',     2403,   33),  -- Âmago de Cavalo Leve B
    ('Simio_Anciao',     4026,   30),  -- Moeda de Prata(1Mi)
    -- Amon e Kalintz
    ('Soldado_Amon',     1357,    0),  -- Calça Guardiã (Arch)
    ('Soldado_Amon',     1359,    0),  -- Botas Guardiãs (Arch)
    ('Soldado_Amon',     1507,    0),  -- Calça da Destruição (Arch)
    ('Soldado_Amon',     1509,    0),  -- Botas da Destruição (Arch)
    ('Soldado_Amon',     1657,    0),  -- Calça Rake (Arch)
    ('Soldado_Amon',     1659,    0),  -- Botas Rake (Arch)
    ('Soldado_Amon',     1711,    0),  -- Hophlon (Arch)
    ('Soldado_Amon',     2400,    0),  -- Âmago de Andaluz N (Andaluz)
    ('Soldado_Amon',     2405,    0),  -- Âmago de Andaluz B (Andaluz)
    ('Soldado_Amon',     3140,    0),  -- Pedra da Luz (Pedra da Luz)
    ('Soldado_Amon',      412,  100),  -- Poeira de Oriharucon
    ('Soldado_Amon',      413,   50),  -- Poeira de Lactolerium
    ('Soldado_Amon',      419,  100),  -- Resto de Oriharucon
    ('Soldado_Amon',      420,   50),  -- Resto de Lactolerium
    ('Soldado_Amon',     2314,    5),  -- Ovo de Cavalo Equip B
    ('Soldado_Amon',     2404,   33),  -- Âmago de Cavalo Equip B
    ('Soldado_Amon',     4026,   30),  -- Moeda de Prata(1Mi)
    ('Capitao_Amon',     1506,    0),  -- Armadura da Destruição (Arch)
    ('Capitao_Amon',     1507,    0),  -- Calça da Destruição (Arch)
    ('Capitao_Amon',     1656,    0),  -- Armadura Rake (Arch)
    ('Capitao_Amon',     1657,    0),  -- Calça Rake (Arch)
    ('Capitao_Amon',     3140,    0),  -- Pedra da Luz (Pedra da Luz)
    ('Capitao_Amon',      412,  100),  -- Poeira de Oriharucon
    ('Capitao_Amon',      413,   50),  -- Poeira de Lactolerium
    ('Capitao_Amon',      419,  100),  -- Resto de Oriharucon
    ('Capitao_Amon',      420,   50),  -- Resto de Lactolerium
    ('Capitao_Amon',     2314,    5),  -- Ovo de Cavalo Equip B
    ('Capitao_Amon',     2404,   33),  -- Âmago de Cavalo Equip B
    ('Capitao_Amon',     4026,   30),  -- Moeda de Prata(1Mi)
    ('Guerreiro_Amon',   1509,    0),  -- Botas da Destruição (Arch)
    ('Guerreiro_Amon',   1659,    0),  -- Botas Rake (Arch)
    ('Guerreiro_Amon',   2309,    0),  -- Ovo de Cavalo Equip N (montaria N (o Gelo é das brancas))
    ('Guerreiro_Amon',    412,  100),  -- Poeira de Oriharucon
    ('Guerreiro_Amon',    413,   50),  -- Poeira de Lactolerium
    ('Guerreiro_Amon',    419,  100),  -- Resto de Oriharucon
    ('Guerreiro_Amon',    420,   50),  -- Resto de Lactolerium
    ('Guerreiro_Amon',   2314,    5),  -- Ovo de Cavalo Equip B
    ('Guerreiro_Amon',   2404,   33),  -- Âmago de Cavalo Equip B
    ('Guerreiro_Amon',   4026,   30),  -- Moeda de Prata(1Mi)
    ('Homem_Kalintz',     811,    0),  -- Balmung (Arch)
    ('Homem_Kalintz',     841,    0),  -- Khyrius (Arch)
    ('Homem_Kalintz',     871,    0),  -- Caliburn (Arch)
    ('Homem_Kalintz',     912,    0),  -- Thrasytes (Arch)
    ('Homem_Kalintz',     937,    0),  -- Basileus (Arch)
    ('Homem_Kalintz',    2310,    0),  -- Ovo de Andaluz N (Andaluz)
    ('Homem_Kalintz',    2315,    0),  -- Ovo de Andaluz B (Andaluz)
    ('Homem_Kalintz',    2400,    0),  -- Âmago de Andaluz N (Andaluz)
    ('Homem_Kalintz',    2405,    0),  -- Âmago de Andaluz B (Andaluz)
    ('Homem_Kalintz',     412,  100),  -- Poeira de Oriharucon
    ('Homem_Kalintz',     413,   50),  -- Poeira de Lactolerium
    ('Homem_Kalintz',     419,  100),  -- Resto de Oriharucon
    ('Homem_Kalintz',     420,   50),  -- Resto de Lactolerium
    ('Homem_Kalintz',    1712,   10),  -- Escudo do Guardião
    ('Homem_Kalintz',    3551,   10),  -- Asa Draconiana
    ('Homem_Kalintz',    3556,   10),  -- Arco Guardião
    ('Homem_Kalintz',    3561,   10),  -- Dianus
    ('Homem_Kalintz',    3566,   10),  -- Foice Platinada
    ('Homem_Kalintz',    3571,   10),  -- Vingadora
    ('Homem_Kalintz',    3576,   10),  -- Karikas
    ('Homem_Kalintz',    3581,   10),  -- Força Eterna
    ('Homem_Kalintz',    3582,   10),  -- Cajado Caótico
    ('Homem_Kalintz',    3591,   10),  -- Éden
    ('Homem_Kalintz',    3596,   10),  -- Demolidor Celestial
    ('Homem_Kalintz',    4026,   30),  -- Moeda de Prata(1Mi)
    ('Mulher_Kalintz',    811,    0),  -- Balmung (Arch)
    ('Mulher_Kalintz',    826,    0),  -- Skytalos (Arch)
    ('Mulher_Kalintz',    856,    0),  -- Gleipnir (Arch)
    ('Mulher_Kalintz',    886,    0),  -- Hermai (Arch)
    ('Mulher_Kalintz',    903,    0),  -- Eirenus (Arch)
    ('Mulher_Kalintz',    904,    0),  -- Neorion (Arch)
    ('Mulher_Kalintz',   2310,    0),  -- Ovo de Andaluz N (Andaluz)
    ('Mulher_Kalintz',   2315,    0),  -- Ovo de Andaluz B (Andaluz)
    ('Mulher_Kalintz',   2400,    0),  -- Âmago de Andaluz N (Andaluz)
    ('Mulher_Kalintz',   2405,    0),  -- Âmago de Andaluz B (Andaluz)
    ('Mulher_Kalintz',    412,  100),  -- Poeira de Oriharucon
    ('Mulher_Kalintz',    413,   50),  -- Poeira de Lactolerium
    ('Mulher_Kalintz',    419,  100),  -- Resto de Oriharucon
    ('Mulher_Kalintz',    420,   50),  -- Resto de Lactolerium
    ('Mulher_Kalintz',   1712,   10),  -- Escudo do Guardião
    ('Mulher_Kalintz',   3551,   10),  -- Asa Draconiana
    ('Mulher_Kalintz',   3556,   10),  -- Arco Guardião
    ('Mulher_Kalintz',   3561,   10),  -- Dianus
    ('Mulher_Kalintz',   3566,   10),  -- Foice Platinada
    ('Mulher_Kalintz',   3571,   10),  -- Vingadora
    ('Mulher_Kalintz',   3576,   10),  -- Karikas
    ('Mulher_Kalintz',   3581,   10),  -- Força Eterna
    ('Mulher_Kalintz',   3582,   10),  -- Cajado Caótico
    ('Mulher_Kalintz',   3591,   10),  -- Éden
    ('Mulher_Kalintz',   3596,   10),  -- Demolidor Celestial
    ('Mulher_Kalintz',   4026,   30),  -- Moeda de Prata(1Mi)
    -- Vila Amald
    ('Mago_Amald',        412,  100),  -- Poeira de Oriharucon
    ('Mago_Amald',        413,   50),  -- Poeira de Lactolerium
    ('Mago_Amald',        419,  100),  -- Resto de Oriharucon
    ('Mago_Amald',        420,   50),  -- Resto de Lactolerium
    ('Mago_Amald',       1225,   15),  -- Elmo Mortal
    ('Mago_Amald',       1226,   15),  -- Armadura Mortal
    ('Mago_Amald',       1227,   15),  -- Calça Mortal
    ('Mago_Amald',       1228,   15),  -- Monaplas Mortais
    ('Mago_Amald',       1229,   15),  -- Botas Mortais
    ('Mago_Amald',       1360,   15),  -- Elmo Templário
    ('Mago_Amald',       1361,   15),  -- Túnica Templária
    ('Mago_Amald',       1362,   15),  -- Calça Templária
    ('Mago_Amald',       1363,   15),  -- Manoplas Templárias
    ('Mago_Amald',       1364,   15),  -- Botas Templárias
    ('Mago_Amald',       1510,   15),  -- Elmo do Corvo
    ('Mago_Amald',       1511,   15),  -- Armadura do Corvo
    ('Mago_Amald',       1512,   15),  -- Calça do Corvo
    ('Mago_Amald',       1513,   15),  -- Manoplas do Corvo
    ('Mago_Amald',       1514,   15),  -- Botas do Corvo
    ('Mago_Amald',       1660,   15),  -- Elmo Legionário
    ('Mago_Amald',       1661,   15),  -- Armadura Legionária
    ('Mago_Amald',       1662,   15),  -- Calça Legionária
    ('Mago_Amald',       1663,   15),  -- Manoplas Legionárias
    ('Mago_Amald',       1664,   15),  -- Botas Legionárias
    ('Mago_Amald',       4026,   30),  -- Moeda de Prata(1Mi)
    ('Shama_Amald',      2316,    0),  -- Ovo de Fenrir (Fenrir e montaria alta)
    ('Shama_Amald',      2321,    0),  -- Ovo de Unicornio (Fenrir e montaria alta)
    ('Shama_Amald',      2406,    0),  -- Âmago de Fenrir (Fenrir e montaria alta)
    ('Shama_Amald',      2411,    0),  -- Âmago de Unicórnio (Fenrir e montaria alta)
    ('Shama_Amald',       412,  100),  -- Poeira de Oriharucon
    ('Shama_Amald',       413,   50),  -- Poeira de Lactolerium
    ('Shama_Amald',       419,  100),  -- Resto de Oriharucon
    ('Shama_Amald',       420,   50),  -- Resto de Lactolerium
    ('Shama_Amald',      1225,   15),  -- Elmo Mortal
    ('Shama_Amald',      1226,   15),  -- Armadura Mortal
    ('Shama_Amald',      1227,   15),  -- Calça Mortal
    ('Shama_Amald',      1228,   15),  -- Monaplas Mortais
    ('Shama_Amald',      1229,   15),  -- Botas Mortais
    ('Shama_Amald',      1360,   15),  -- Elmo Templário
    ('Shama_Amald',      1361,   15),  -- Túnica Templária
    ('Shama_Amald',      1362,   15),  -- Calça Templária
    ('Shama_Amald',      1363,   15),  -- Manoplas Templárias
    ('Shama_Amald',      1364,   15),  -- Botas Templárias
    ('Shama_Amald',      1510,   15),  -- Elmo do Corvo
    ('Shama_Amald',      1511,   15),  -- Armadura do Corvo
    ('Shama_Amald',      1512,   15),  -- Calça do Corvo
    ('Shama_Amald',      1513,   15),  -- Manoplas do Corvo
    ('Shama_Amald',      1514,   15),  -- Botas do Corvo
    ('Shama_Amald',      1660,   15),  -- Elmo Legionário
    ('Shama_Amald',      1661,   15),  -- Armadura Legionária
    ('Shama_Amald',      1662,   15),  -- Calça Legionária
    ('Shama_Amald',      1663,   15),  -- Manoplas Legionárias
    ('Shama_Amald',      1664,   15),  -- Botas Legionárias
    ('Shama_Amald',      4026,   30),  -- Moeda de Prata(1Mi)
    ('Templario_Amald',  2316,    0),  -- Ovo de Fenrir (Fenrir e montaria alta)
    ('Templario_Amald',  2321,    0),  -- Ovo de Unicornio (Fenrir e montaria alta)
    ('Templario_Amald',  2406,    0),  -- Âmago de Fenrir (Fenrir e montaria alta)
    ('Templario_Amald',  2411,    0),  -- Âmago de Unicórnio (Fenrir e montaria alta)
    ('Templario_Amald',   412,  100),  -- Poeira de Oriharucon
    ('Templario_Amald',   413,   50),  -- Poeira de Lactolerium
    ('Templario_Amald',   419,  100),  -- Resto de Oriharucon
    ('Templario_Amald',   420,   50),  -- Resto de Lactolerium
    ('Templario_Amald',  1225,   15),  -- Elmo Mortal
    ('Templario_Amald',  1226,   15),  -- Armadura Mortal
    ('Templario_Amald',  1227,   15),  -- Calça Mortal
    ('Templario_Amald',  1228,   15),  -- Monaplas Mortais
    ('Templario_Amald',  1229,   15),  -- Botas Mortais
    ('Templario_Amald',  1360,   15),  -- Elmo Templário
    ('Templario_Amald',  1361,   15),  -- Túnica Templária
    ('Templario_Amald',  1362,   15),  -- Calça Templária
    ('Templario_Amald',  1363,   15),  -- Manoplas Templárias
    ('Templario_Amald',  1364,   15),  -- Botas Templárias
    ('Templario_Amald',  1510,   15),  -- Elmo do Corvo
    ('Templario_Amald',  1511,   15),  -- Armadura do Corvo
    ('Templario_Amald',  1512,   15),  -- Calça do Corvo
    ('Templario_Amald',  1513,   15),  -- Manoplas do Corvo
    ('Templario_Amald',  1514,   15),  -- Botas do Corvo
    ('Templario_Amald',  1660,   15),  -- Elmo Legionário
    ('Templario_Amald',  1661,   15),  -- Armadura Legionária
    ('Templario_Amald',  1662,   15),  -- Calça Legionária
    ('Templario_Amald',  1663,   15),  -- Manoplas Legionárias
    ('Templario_Amald',  1664,   15),  -- Botas Legionárias
    ('Templario_Amald',  4026,   30),  -- Moeda de Prata(1Mi)
    ('Ranger_Amald',      412,  100),  -- Poeira de Oriharucon
    ('Ranger_Amald',      413,   50),  -- Poeira de Lactolerium
    ('Ranger_Amald',      419,  100),  -- Resto de Oriharucon
    ('Ranger_Amald',      420,   50),  -- Resto de Lactolerium
    ('Ranger_Amald',     1225,   15),  -- Elmo Mortal
    ('Ranger_Amald',     1226,   15),  -- Armadura Mortal
    ('Ranger_Amald',     1227,   15),  -- Calça Mortal
    ('Ranger_Amald',     1228,   15),  -- Monaplas Mortais
    ('Ranger_Amald',     1229,   15),  -- Botas Mortais
    ('Ranger_Amald',     1360,   15),  -- Elmo Templário
    ('Ranger_Amald',     1361,   15),  -- Túnica Templária
    ('Ranger_Amald',     1362,   15),  -- Calça Templária
    ('Ranger_Amald',     1363,   15),  -- Manoplas Templárias
    ('Ranger_Amald',     1364,   15),  -- Botas Templárias
    ('Ranger_Amald',     1510,   15),  -- Elmo do Corvo
    ('Ranger_Amald',     1511,   15),  -- Armadura do Corvo
    ('Ranger_Amald',     1512,   15),  -- Calça do Corvo
    ('Ranger_Amald',     1513,   15),  -- Manoplas do Corvo
    ('Ranger_Amald',     1514,   15),  -- Botas do Corvo
    ('Ranger_Amald',     1660,   15),  -- Elmo Legionário
    ('Ranger_Amald',     1661,   15),  -- Armadura Legionária
    ('Ranger_Amald',     1662,   15),  -- Calça Legionária
    ('Ranger_Amald',     1663,   15),  -- Manoplas Legionárias
    ('Ranger_Amald',     1664,   15),  -- Botas Legionárias
    ('Ranger_Amald',     4026,   30),  -- Moeda de Prata(1Mi)
    -- Fortaleza
    ('Valquiria_Rosen',  2315,    0),  -- Ovo de Andaluz B (Andaluz)
    ('Valquiria_Rosen',  2322,    0),  -- Ovo de Pegasus (Fenrir e montaria alta)
    ('Valquiria_Rosen',  2412,    0),  -- Âmago de Pégasus (Fenrir e montaria alta)
    ('Valquiria_Rosen',   412,  100),  -- Poeira de Oriharucon
    ('Valquiria_Rosen',   413,   50),  -- Poeira de Lactolerium
    ('Valquiria_Rosen',   419,  100),  -- Resto de Oriharucon
    ('Valquiria_Rosen',   420,   50),  -- Resto de Lactolerium
    ('Valquiria_Rosen',  2206,   16),  -- Chapéu de Mytril(Le)
    ('Valquiria_Rosen',  2207,   16),  -- Túnica de Mytril(Le)
    ('Valquiria_Rosen',  2208,   16),  -- Calça de Mytril(Le)
    ('Valquiria_Rosen',  2209,   16),  -- Luvas de Mytril(Le)
    ('Valquiria_Rosen',  2210,   16),  -- Botas de Mytril(Le)
    ('Valquiria_Rosen',  2246,   16),  -- Chapéu de Teia(Le)
    ('Valquiria_Rosen',  2247,   16),  -- Peitoral de Teia(Le)
    ('Valquiria_Rosen',  2248,   16),  -- Calça de Teia(Le)
    ('Valquiria_Rosen',  2249,   16),  -- Braçadeira de Teia(Le)
    ('Valquiria_Rosen',  2250,   16),  -- Botas de Teia(Le)
    ('Valquiria_Rosen',  4026,   30),  -- Moeda de Prata(1Mi)
    ('Valquiria_Tina',    886,    0),  -- Hermai (Arch)
    ('Valquiria_Tina',   2323,    0),  -- Ovo de Unisus (Fenrir e montaria alta)
    ('Valquiria_Tina',   2406,    0),  -- Âmago de Fenrir (Fenrir e montaria alta)
    ('Valquiria_Tina',   2413,    0),  -- Âmago de Unisus (Fenrir e montaria alta)
    ('Valquiria_Tina',   3140,    0),  -- Pedra da Luz (Pedra da Luz)
    ('Valquiria_Tina',    412,  100),  -- Poeira de Oriharucon
    ('Valquiria_Tina',    413,   50),  -- Poeira de Lactolerium
    ('Valquiria_Tina',    419,  100),  -- Resto de Oriharucon
    ('Valquiria_Tina',    420,   50),  -- Resto de Lactolerium
    ('Valquiria_Tina',   2206,   16),  -- Chapéu de Mytril(Le)
    ('Valquiria_Tina',   2207,   16),  -- Túnica de Mytril(Le)
    ('Valquiria_Tina',   2208,   16),  -- Calça de Mytril(Le)
    ('Valquiria_Tina',   2209,   16),  -- Luvas de Mytril(Le)
    ('Valquiria_Tina',   2210,   16),  -- Botas de Mytril(Le)
    ('Valquiria_Tina',   2246,   16),  -- Chapéu de Teia(Le)
    ('Valquiria_Tina',   2247,   16),  -- Peitoral de Teia(Le)
    ('Valquiria_Tina',   2248,   16),  -- Calça de Teia(Le)
    ('Valquiria_Tina',   2249,   16),  -- Braçadeira de Teia(Le)
    ('Valquiria_Tina',   2250,   16),  -- Botas de Teia(Le)
    ('Valquiria_Tina',   4026,   30),  -- Moeda de Prata(1Mi)
    ('Tita_Berserker',   2321,    0),  -- Ovo de Unicornio (Fenrir e montaria alta)
    ('Tita_Berserker',   2413,    0),  -- Âmago de Unisus (Fenrir e montaria alta)
    ('Tita_Berserker',   3140,    0),  -- Pedra da Luz (Pedra da Luz)
    ('Tita_Berserker',    412,  100),  -- Poeira de Oriharucon
    ('Tita_Berserker',    413,   50),  -- Poeira de Lactolerium
    ('Tita_Berserker',    419,  100),  -- Resto de Oriharucon
    ('Tita_Berserker',    420,   50),  -- Resto de Lactolerium
    ('Tita_Berserker',   4026,   30),  -- Moeda de Prata(1Mi)
    ('Guarda_Beriel',    2310,    0),  -- Ovo de Andaluz N (Andaluz)
    ('Guarda_Beriel',    2322,    0),  -- Ovo de Pegasus (Fenrir e montaria alta)
    ('Guarda_Beriel',    2412,    0),  -- Âmago de Pégasus (Fenrir e montaria alta)
    ('Guarda_Beriel',     412,  100),  -- Poeira de Oriharucon
    ('Guarda_Beriel',     413,   50),  -- Poeira de Lactolerium
    ('Guarda_Beriel',     419,  100),  -- Resto de Oriharucon
    ('Guarda_Beriel',     420,   50),  -- Resto de Lactolerium
    ('Guarda_Beriel',    2186,   16),  -- Elmo Embutido(Le)
    ('Guarda_Beriel',    2187,   16),  -- Armadura Embutida(Le)
    ('Guarda_Beriel',    2188,   16),  -- Calça Embutida(Le)
    ('Guarda_Beriel',    2189,   16),  -- Manoplas Embutidas(Le)
    ('Guarda_Beriel',    2190,   16),  -- Botas Embutidas(Le)
    ('Guarda_Beriel',    2226,   16),  -- Elmo Elemental(Le)
    ('Guarda_Beriel',    2227,   16),  -- Armadura Elemental(Le)
    ('Guarda_Beriel',    2228,   16),  -- Calça Elemental(Le)
    ('Guarda_Beriel',    2229,   16),  -- Manoplas Elementais(Le)
    ('Guarda_Beriel',    2230,   16),  -- Botas Elementais(Le)
    ('Guarda_Beriel',    4026,   30),  -- Moeda de Prata(1Mi)
    ('Berserker_Ref',    2322,    0),  -- Ovo de Pegasus (Fenrir e montaria alta)
    ('Berserker_Ref',    2412,    0),  -- Âmago de Pégasus (Fenrir e montaria alta)
    ('Berserker_Ref',     412,  100),  -- Poeira de Oriharucon
    ('Berserker_Ref',     413,   50),  -- Poeira de Lactolerium
    ('Berserker_Ref',     419,  100),  -- Resto de Oriharucon
    ('Berserker_Ref',     420,   50),  -- Resto de Lactolerium
    ('Berserker_Ref',    4026,   30),  -- Moeda de Prata(1Mi)
    -- Chefe
    ('Verid',            1711,    0),  -- Hophlon (Arch)
    ('Verid',            2399,    0),  -- Âmago de Cavalo Equip N (montaria N (o Gelo é das brancas))
    ('Verid',            2400,    0),  -- Âmago de Andaluz N (Andaluz)
    ('Verid',            2405,    0),  -- Âmago de Andaluz B (Andaluz)
    ('Verid',            2411,    0),  -- Âmago de Unicórnio (Fenrir e montaria alta)
    ('Verid',            2412,    0),  -- Âmago de Pégasus (Fenrir e montaria alta)
    ('Verid',            2416,    0),  -- Âmago de Grifo Sang (Fenrir e montaria alta)
    ('Verid_',           1711,    0),  -- Hophlon (Arch)
    ('Verid_',           2399,    0),  -- Âmago de Cavalo Equip N (montaria N (o Gelo é das brancas))
    ('Verid_',           2400,    0),  -- Âmago de Andaluz N (Andaluz)
    ('Verid_',           2405,    0),  -- Âmago de Andaluz B (Andaluz)
    ('Verid_',           2411,    0),  -- Âmago de Unicórnio (Fenrir e montaria alta)
    ('Verid_',           2412,    0),  -- Âmago de Pégasus (Fenrir e montaria alta)
    ('Verid_',           2416,    0)  -- Âmago de Grifo Sang (Fenrir e montaria alta)
ON CONFLICT (mob, item) DO UPDATE SET chance = EXCLUDED.chance, updated_at = now();

UPDATE drop_rule_meta SET version = version + 1 WHERE id = TRUE;
