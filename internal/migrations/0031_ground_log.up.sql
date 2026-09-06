-- 0031_ground_log — o que passou pelo chão.
--
-- O registro de trocas (0025) já dizia o próprio buraco: "um item entregue
-- largando no chão para o outro pegar. getItem entrega um item de chão a
-- qualquer um a três casas de distância, sem checar dono e sem log, então um
-- golpista determinado tem um caminho que não deixa nada." Esta tabela é esse
-- nada.
--
-- Serve a duas perguntas diferentes. Na primeira, alguém abre chamado dizendo
-- que combinou de largar e o outro pegou correndo: aí o que se procura é um
-- nome e uma hora. Na segunda, o censo de itens vê um número subir e precisa
-- saber se aquilo caiu de monstro, foi entregue, ou apareceu do nada — sem isto
-- o chão é um buraco por onde item entra e sai sem explicação.
--
-- chao_id é a casa do item no vetor de chão do servidor. Ele é reaproveitado ao
-- longo do tempo, então não identifica um item para sempre — mas emparelha um
-- "largou" com o "pegou" que veio logo depois, que é exatamente a pergunta que
-- se faz olhando isto.
--
-- Sem chave estrangeira para character ou account, pelo mesmo motivo do 0025:
-- DeleteCharacter é do jogador e apaga a linha de verdade, então CASCADE
-- deixaria o suspeito apagar a prova.
--
-- PRAZO de 30 dias, e este é por VOLUME, não por privacidade: largar coisa no
-- chão é constante, e sem prazo a tabela cresce para sempre guardando poção
-- descartada de dois anos atrás. Trinta dias cobre com folga o tempo entre um
-- golpe e o chamado que ele gera.

CREATE TABLE ground_log (
    id          BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    ocorrido_em TIMESTAMPTZ NOT NULL DEFAULT now(),
    expira_em   TIMESTAMPTZ NOT NULL,

    -- 'largou' ou 'pegou'. Duas linhas por item que troca de mão pelo chão.
    acao TEXT NOT NULL CHECK (acao IN ('largou', 'pegou')),

    -- Quem. Copiados, não referenciados.
    conta_id  BIGINT,
    char_nome TEXT NOT NULL,

    -- O quê, com os efeitos da instância: é o que distingue a espada refinada
    -- da espada comum quando as duas têm o mesmo índice.
    item_index SMALLINT NOT NULL,
    eff1  SMALLINT NOT NULL DEFAULT 0,
    effv1 SMALLINT NOT NULL DEFAULT 0,
    eff2  SMALLINT NOT NULL DEFAULT 0,
    effv2 SMALLINT NOT NULL DEFAULT 0,
    eff3  SMALLINT NOT NULL DEFAULT 0,
    effv3 SMALLINT NOT NULL DEFAULT 0,

    -- Onde, e em que casa do chão. A casa é o que emparelha largou/pegou.
    pos_x   INTEGER NOT NULL DEFAULT 0,
    pos_y   INTEGER NOT NULL DEFAULT 0,
    chao_id INTEGER NOT NULL DEFAULT 0
);

-- A busca que o atendimento faz: um nome e uma hora.
CREATE INDEX ground_log_char_idx ON ground_log (char_nome, ocorrido_em DESC);

-- Emparelhar o "pegou" com o "largou" que veio antes, na mesma casa.
CREATE INDEX ground_log_chao_idx ON ground_log (chao_id, ocorrido_em DESC);

-- Percorrer o período, e varrer o prazo.
CREATE INDEX ground_log_ocorrido_idx ON ground_log (ocorrido_em DESC);
CREATE INDEX ground_log_expira_idx ON ground_log (expira_em);
