-- 0165_receita_de_bloco — mexer num bloco do NPCGener sem reiniciar.
--
-- O NPCGener.txt vem dentro da imagem: mudar um bloco no arquivo é deploy, e deploy
-- derruba todo mundo que está jogando. A equipe ainda vai mexer em muitas zonas de
-- caça, então a receita de um bloco passa a poder morar aqui, como a Mesa de Drops:
-- a linha SUBSTITUI o que o arquivo diz daquele bloco, e apagar a linha devolve o
-- bloco ao arquivo.
--
-- A linha é a receita INTEIRA, não campo a campo: o painel preenche o formulário com
-- o que o arquivo diz e grava tudo. Assim uma linha lida sozinha já responde "o que
-- este bloco gera", sem precisar do arquivo ao lado.
--
-- ÍNDICE: o mesmo número de /gm npc e da tela /blocos (Entity.GenIndex). Abaixo do
-- total de blocos do arquivo, a linha troca um bloco que existe. Bloco NOVO, criado
-- pelo painel, mora de 20000 para cima — longe do fim do arquivo, que cresce toda
-- semana: um bloco acrescentado no .txt não pode cair em cima de um do banco. O
-- teto é o do int16 que carrega o índice em cada mob (MobSpawn.GenIndex).
--
-- As falas de luta e de morte (FightAction/DieAction) não moram aqui: continuam as
-- do arquivo, e bloco novo não tem nenhuma.
--
-- VALE AO VIVO: o tmServer pergunta a versão a cada ~15 s. O que muda vale para o
-- próximo que nascer; quem já está no mapa fica até morrer. `renovar` é o botão
-- "trocar os vivos agora": o painel soma 1, e o jogo, ao ver o número mudar, tira
-- os vivos do bloco e gera de novo pela receita nova.

CREATE TABLE npc_generator_recipe (
    generator_index INTEGER     PRIMARY KEY
                                CHECK (generator_index >= 0 AND generator_index <= 32767),
    leader          TEXT        NOT NULL CHECK (leader <> ''),
    follower        TEXT        NOT NULL DEFAULT '',
    minute_generate INTEGER     NOT NULL,
    -- O arquivo tem grupos de até 49 (a geração corta em 12 seguidores): o teto
    -- aqui é folgado de propósito, para regravar um bloco do arquivo sem recusa.
    min_group       INTEGER     NOT NULL CHECK (min_group BETWEEN 0 AND 100),
    max_group       INTEGER     NOT NULL CHECK (max_group BETWEEN 0 AND 100),
    max_num_mob     INTEGER     NOT NULL CHECK (max_num_mob BETWEEN -1 AND 1000),
    route_type      SMALLINT    NOT NULL DEFAULT 0 CHECK (route_type BETWEEN 0 AND 255),
    formation       SMALLINT    NOT NULL DEFAULT 0 CHECK (formation BETWEEN 0 AND 4),
    -- Os cinco pontos na ordem do legado: Start, Segment1..3, Dest (npcgener).
    seg_x           SMALLINT[]  NOT NULL CHECK (array_length(seg_x, 1) = 5),
    seg_y           SMALLINT[]  NOT NULL CHECK (array_length(seg_y, 1) = 5),
    seg_range       SMALLINT[]  NOT NULL CHECK (array_length(seg_range, 1) = 5),
    seg_wait        SMALLINT[]  NOT NULL CHECK (array_length(seg_wait, 1) = 5),
    renovar         BIGINT      NOT NULL DEFAULT 0,
    nota            TEXT        NOT NULL DEFAULT '',
    updated_by      BIGINT      REFERENCES account(id) ON DELETE SET NULL,
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE npc_generator_recipe_meta (
    id      BOOLEAN PRIMARY KEY DEFAULT TRUE CHECK (id),
    version BIGINT  NOT NULL DEFAULT 0
);

INSERT INTO npc_generator_recipe_meta (id, version) VALUES (TRUE, 0);
