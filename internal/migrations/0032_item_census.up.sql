-- 0032_item_census — quantas unidades de cada item existem, por dia.
--
-- Item no WYD não tem identidade: um item é um índice mais três pares de
-- efeito, e duas unidades iguais são indistinguíveis. Não dá para perguntar
-- "este item aqui é cópia de qual". Dá para perguntar outra coisa: quantos
-- existiam ontem, quantos existem hoje, e o que explica a diferença.
--
-- Conta por ÍNDICE E POR REFINO (EF_SANC, efeito 43), não só por índice. Isso
-- muda tudo na prática: ninguém dupa espada +0. Trinta unidades a mais do
-- índice 1100 no refino 0 é gente farmando; catorze a mais no refino 11 é
-- outra conversa. Sem separar o refino, o barulho do item comum afoga o sinal.
--
-- LIMITE, dito aqui para não ser descoberto na tela: o censo só enxerga o que
-- foi salvo. Não existe save periódico de personagem — item chega no Postgres
-- no logout. Quem está online há três dias tem o inventário de três dias atrás
-- no banco. A contagem de hoje é a do mundo de ontem à noite, e é por isso que
-- item_census_meta guarda a HORA da contagem: quem lê precisa saber de quando
-- é a foto.
--
-- Também não distingue "nasceu" de "voltou a aparecer": um item que estava com
-- alguém offline há um mês entra na contagem só quando aquela conta salva. Por
-- isso a leitura útil é a tendência ao longo de dias, não o pulo de um dia.
--
-- SEM PRAZO de validade, ao contrário do registro de chão (0031). Lá o prazo
-- existe porque a tabela cresce com o movimento do servidor e não tem fim. Aqui
-- ela cresce com o calendário: uma linha por índice e refino por dia, uns
-- milhares por dia no pior caso. E o histórico é justamente o produto — jogar
-- fora o ano passado é jogar fora a comparação entre temporadas.

CREATE TABLE item_census (
    dia        DATE NOT NULL,
    item_index SMALLINT NOT NULL,
    -- Nível de refino (EF_SANC). Zero quando o item não tem o efeito.
    sanc       SMALLINT NOT NULL,

    unidades   INTEGER NOT NULL,
    -- Onde estavam, para separar "está em uso" de "está guardado". Um monte
    -- que só cresce no baú é diferente de um que cresce equipado.
    equipados  INTEGER NOT NULL DEFAULT 0,
    mochila    INTEGER NOT NULL DEFAULT 0,
    bau        INTEGER NOT NULL DEFAULT 0,

    PRIMARY KEY (dia, item_index, sanc)
);

-- A leitura da tela: percorrer um índice ao longo dos dias.
CREATE INDEX item_census_item_idx ON item_census (item_index, sanc, dia DESC);

-- item_census_meta existe por dois motivos, e o segundo é o que importa.
--
-- O primeiro: quem lê a tela precisa da hora da contagem, não só do dia.
--
-- O segundo: é o que diz se a foto de hoje já foi tirada. Sem ela, "existe
-- linha em item_census com dia = hoje?" responde "não" num mundo vazio, e a
-- rotina tentaria contar de novo a cada seis horas para sempre.
CREATE TABLE item_census_meta (
    dia        DATE PRIMARY KEY,
    contado_em TIMESTAMPTZ NOT NULL DEFAULT now(),
    -- Quantas unidades ao todo, somando tudo. Serve de conferência rápida: se
    -- este número despenca, alguém apagou conta, não sumiu item do mundo.
    unidades   INTEGER NOT NULL DEFAULT 0,
    -- Quantas linhas (índice+refino) diferentes a foto tem.
    variedades INTEGER NOT NULL DEFAULT 0
);
