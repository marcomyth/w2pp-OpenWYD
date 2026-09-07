-- 0034_chat_log — o que foi dito.
--
-- Serve a uma pergunta só, e é a que o atendimento faz todo dia: alguém abre
-- chamado dizendo que foi xingado, ameaçado ou enganado por conversa, e hoje
-- não existe nada para olhar. A denúncia (0028) guarda o momento e quem estava
-- perto; não guarda o que foi dito.
--
-- PRAZO de 30 dias, decidido pela dona do servidor, com a instrução de cair
-- para 20 se a tabela pesar.
--
-- E por isso NÃO existe coluna de vencimento aqui, ao contrário do registro de
-- chão (0031). Lá o prazo é gravado em cada linha na hora da escrita, o que é
-- ótimo enquanto ele não muda — mas mudaria só para as linhas novas, e trocar
-- 30 por 20 não encolheria nada do que já está gravado. Aqui a varredura
-- compara `ocorrido_em` com o prazo VIGENTE, então baixar o número encolhe a
-- tabela na varredura seguinte. É o que foi pedido.
--
-- Isto é conversa de gente. Duas consequências que o código tem de honrar:
-- o prazo é a proteção (passou de 30 dias, some, e não existe jeito de
-- recuperar), e toda LEITURA desta tabela pelo painel é registrada na
-- auditoria — ler mensagem privada de um jogador é coisa que tem de deixar
-- rastro de quem leu.
--
-- Sem chave estrangeira para conta ou personagem, pelo mesmo motivo dos outros
-- registros: DeleteCharacter é do jogador, e CASCADE deixaria o suspeito apagar
-- a prova apagando o personagem.

CREATE TABLE chat_log (
    id          BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    ocorrido_em TIMESTAMPTZ NOT NULL DEFAULT now(),

    -- 'publico' (quem estava por perto ouviu) ou 'sussurro' (só o alvo).
    tipo TEXT NOT NULL CHECK (tipo IN ('publico', 'sussurro')),

    -- Quem falou. Copiados, não referenciados.
    conta_id  BIGINT,
    char_nome TEXT NOT NULL,

    -- Para quem, no sussurro. NULL no público, que não tem destinatário.
    alvo_nome TEXT,

    texto TEXT NOT NULL,

    -- Onde estava. É o que cruza uma fala com uma denúncia, que também guarda
    -- posição (0028).
    pos_x INTEGER NOT NULL DEFAULT 0,
    pos_y INTEGER NOT NULL DEFAULT 0
);

-- A busca do atendimento: um nome e um período.
CREATE INDEX chat_log_char_idx ON chat_log (char_nome, ocorrido_em DESC);

-- Quem RECEBEU o sussurro — a outra metade da mesma pergunta, e a que responde
-- "o que mandaram para mim". Parcial porque público não tem alvo.
CREATE INDEX chat_log_alvo_idx ON chat_log (alvo_nome, ocorrido_em DESC)
    WHERE alvo_nome IS NOT NULL;

-- Percorrer o período, e varrer o prazo.
CREATE INDEX chat_log_ocorrido_idx ON chat_log (ocorrido_em DESC);

-- chat_log_meta é uma linha só: o que a última varredura fez.
--
-- Existe porque o prazo é uma configuração do servidor de banco, e o painel é
-- outro processo. Sem isto a tela teria de ler a própria variável de ambiente e
-- afirmar um prazo que talvez não seja o que está sendo aplicado. Assim ela
-- mostra o que a varredura de verdade usou.
CREATE TABLE chat_log_meta (
    unico       BOOLEAN PRIMARY KEY DEFAULT true CHECK (unico),
    varrido_em  TIMESTAMPTZ,
    dias        INTEGER NOT NULL DEFAULT 30,
    apagadas    BIGINT NOT NULL DEFAULT 0
);

INSERT INTO chat_log_meta (unico) VALUES (true);
