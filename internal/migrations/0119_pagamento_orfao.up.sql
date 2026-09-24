-- 0119_pagamento_orfao — o dinheiro que entrou e não achou cobrança.
--
-- POR QUE UMA TABELA E NÃO UMA LINHA DE LOG: porque é dinheiro de uma pessoa. Um
-- log é apagado por rotação, não tem fila, ninguém é responsável por ele e não dá
-- para marcar como resolvido. O caminho do aviso de pagamento tinha exatamente
-- esse buraco — ele registrava "aviso para uma referência que não existe" e seguia
-- em frente —, e o que sobrava disso era alguém ter pagado e o servidor ter
-- escrito uma linha que ninguém lê.
--
-- COMO ISTO ACONTECE DE VERDADE, e a primeira é a comum:
--
--  1. CORRIDA NO NASCIMENTO. O servidor pede o Pix à processadora, ela cria, o
--     webhook dela chega ao site e é repassado ANTES de a nossa linha ter gravado
--     o identifier. Aqui o órfão é temporário: a varredura das cobranças abertas
--     encontra o pagamento pouco depois. Fica registrado de propósito, porque
--     órfão que se resolve sozinho e órfão de verdade são indistinguíveis no
--     instante em que chegam.
--  2. A processadora devolveu uma referência que não é de nenhuma cobrança nossa,
--     ou é de uma e o identifier é de outra. Aí é erro de alguém, e a decisão é de
--     uma pessoa: devolver, ou achar a venda na mão.
--  3. Aviso forjado com assinatura válida — o que só é possível se o segredo
--     vazou. Neste caso a linha aqui é a primeira evidência disso.
--
-- NÃO ENTREGA NADA E NÃO DEVOLVE NADA SOZINHA. Gravar é tudo o que ela faz: as
-- três causas pedem respostas diferentes e escolher por conta própria é decidir
-- sobre o dinheiro de outra pessoa.
CREATE TABLE rmt_pagamento_orfao (
    id             BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    -- O identifier DELES, e é a chave de verdade: é o único campo que identifica
    -- o pagamento quando não existe cobrança nossa para apontar.
    --
    -- ÚNICO porque a processadora avisa mais de uma vez sobre o mesmo pagamento.
    -- Sem isto, um webhook repetido viraria uma fila de dez linhas sobre um
    -- pagamento só, e quem olhasse a fila contaria dez problemas.
    identifier     TEXT NOT NULL UNIQUE,
    -- Nulos porque o primeiro aviso pode chegar antes de a processadora saber. O
    -- registro do pagamento é mais importante do que estar completo.
    valor_centavos BIGINT,
    pago_em        TIMESTAMPTZ,
    -- A referência que a PROCESSADORA disse, guardada crua mesmo quando não bate
    -- com nada. Quando o caso é o 2, é ela que aponta para onde olhar.
    referencia_vista TEXT,
    -- O motivo, em texto, escrito por quem gravou. Não é enum: o conjunto de
    -- motivos ainda está crescendo, e um enum errado esconde o caso novo dentro do
    -- "outro".
    motivo         TEXT NOT NULL,
    visto_em       TIMESTAMPTZ NOT NULL DEFAULT now(),
    -- Quem resolveu e quando. Nulo = está na fila.
    resolvido_em   TIMESTAMPTZ,
    resolvido_por  TEXT,
    nota           TEXT
);

-- A fila: o que ainda precisa de gente, mais antigo primeiro.
CREATE INDEX rmt_pagamento_orfao_na_fila
    ON rmt_pagamento_orfao (visto_em) WHERE resolvido_em IS NULL;
