-- 0105_anuncio_e_cobranca_rmt — o anúncio em dinheiro real e cada tentativa de
-- pagamento contra ele.
--
-- SÃO DUAS TABELAS E NÃO UMA, e a razão é o caso que mais importa: a confirmação
-- que chega DEPOIS do cancelamento.
--
--   O comprador A abre o QR e some. A cobrança é cancelada. O comprador B tenta
--   o mesmo item e abre outra. Se cobrança e anúncio fossem a mesma linha, a
--   segunda sobrescreveria a primeira — e quando o Pix do A caísse atrasado, com
--   a referência externa DELE, não haveria linha que respondesse por aquela
--   referência. Dinheiro entrando e o servidor sem saber de que venda era.
--
-- Some a isso que o anúncio some da vitrine quando o vendedor cai, e a cobrança
-- não pode sumir com ninguém: pendurar dinheiro numa coisa com a vida da sessão
-- é o mesmo erro do "nunca persistido" pela terceira vez neste projeto.
--
-- NADA AQUI É APAGADO NEM REAPROVEITADO. As linhas mudam de estado e ficam.

-- rmt_anuncio: o item posto à venda por dinheiro real.
--
-- É da CONTA e não do personagem porque o baú é da conta, e é no slot do baú que
-- mora a marca do escrow (0104).
CREATE TABLE rmt_anuncio (
    id             BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    -- RESTRICT e não CASCADE, de propósito: ver a nota sobre dinheiro abaixo.
    vendedor_conta BIGINT   NOT NULL REFERENCES account(id) ON DELETE RESTRICT,
    cargo_slot     SMALLINT NOT NULL CHECK (cargo_slot BETWEEN 0 AND 127),
    -- Fotografia do item. O escrow impede que ele mude, então em tese isto é
    -- redundante — mas depois da venda o item não está mais no baú, e sem a
    -- fotografia não há como responder "o que exatamente foi vendido" numa
    -- disputa. Custa seis smallint.
    item_index     SMALLINT NOT NULL,
    eff1           SMALLINT NOT NULL DEFAULT 0,
    effv1          SMALLINT NOT NULL DEFAULT 0,
    eff2           SMALLINT NOT NULL DEFAULT 0,
    effv2          SMALLINT NOT NULL DEFAULT 0,
    eff3           SMALLINT NOT NULL DEFAULT 0,
    effv3          SMALLINT NOT NULL DEFAULT 0,
    preco_centavos BIGINT   NOT NULL CHECK (preco_centavos > 0),
    status         SMALLINT NOT NULL,  -- 1=ATIVO, 2=VENDIDO, 3=CANCELADO
    criado_em      TIMESTAMPTZ NOT NULL DEFAULT now(),
    encerrado_em   TIMESTAMPTZ
);

-- NÃO existe estado "oculto". Sumir da vitrine quando o vendedor cai não é
-- estado da linha: a vitrine é lida das barracas VIVAS, então o anúncio sai de
-- lá sozinho. Guardar isso aqui seria gravar no banco o que a sessão já responde.
--
-- E NÃO existe expira_em: quem expira é a cobrança. O anúncio vive até vender ou
-- ser cancelado.

-- Dois anúncios ativos sobre o MESMO slot apontariam para um item só, e a marca
-- do escrow guarda UM id — o segundo nasceria órfão. No banco e não no código,
-- porque invariante de dinheiro que depende de alguém lembrar não é invariante.
CREATE UNIQUE INDEX rmt_anuncio_um_ativo_por_slot
    ON rmt_anuncio (vendedor_conta, cargo_slot) WHERE status = 1;

-- rmt_cobranca: cada TENTATIVA de pagamento contra um anúncio. Várias ao longo
-- do tempo; nenhuma some.
CREATE TABLE rmt_cobranca (
    id                 BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    anuncio_id         BIGINT NOT NULL REFERENCES rmt_anuncio(id) ON DELETE RESTRICT,
    comprador_conta    BIGINT NOT NULL REFERENCES account(id) ON DELETE RESTRICT,
    -- A âncora da idempotência, igual ao donate_topup_order (0010): confirmação
    -- repetida encontra a MESMA linha e não faz nada duas vezes.
    referencia_externa TEXT   NOT NULL UNIQUE,
    -- O valor fica aqui além do preço no anúncio porque uma tentativa anterior
    -- pode ter cobrado outro: cada cobrança guarda o que ELA cobrou.
    valor_centavos     BIGINT NOT NULL CHECK (valor_centavos > 0),
    -- Coluna, e nunca parte de nome de tabela ou de RPC, para cartão reusar a
    -- mesma tabela — a mesma decisão que a 0010 tomou e escreveu.
    metodo             SMALLINT NOT NULL,  -- 1=PIX
    -- 1=ABERTA 2=PAGA 3=CANCELADA 4=EXPIRADA 5=PAGA_SEM_ITEM
    status             SMALLINT NOT NULL,
    criada_em          TIMESTAMPTZ NOT NULL DEFAULT now(),
    expira_em          TIMESTAMPTZ NOT NULL,
    paga_em            TIMESTAMPTZ,
    encerrada_em       TIMESTAMPTZ,
    -- A segunda marca de "já feito". O status PAGA diz que o dinheiro entrou;
    -- esta diz que o item saiu. Sem ela duas confirmações enfileiram duas
    -- entregas e o item sai dobrado.
    --
    -- Chave estrangeira de verdade e não um número solto: se um dia alguém
    -- limpar a caixa postal antiga, é melhor o banco recusar apagar linha
    -- referenciada do que deixar esta marca apontando para o nada.
    entrega_id         BIGINT REFERENCES delivery_queue(id) ON DELETE RESTRICT,
    -- A confirmação que chegou DEPOIS do cancelamento ou da expiração. A regra é
    -- entregar assim mesmo, mas isso é EXCEÇÃO e precisa ser contável: se virar
    -- rotina, o prazo da cobrança está errado. Sem a coluna, a gente descobriria
    -- por reclamação.
    pago_com_atraso    BOOLEAN NOT NULL DEFAULT FALSE
);

-- A invariante que impede dois compradores de pagarem pelo mesmo item e os dois
-- terem razão. No banco, pelo mesmo motivo do índice lá de cima.
CREATE UNIQUE INDEX rmt_cobranca_uma_aberta_por_anuncio
    ON rmt_cobranca (anuncio_id) WHERE status = 1;

-- UMA INVARIANTE QUE NÃO ESTÁ AQUI, E NÃO POR ESQUECIMENTO: o comprador não pode
-- ser o vendedor.
--
-- Não dá para escrevê-la como CHECK porque as duas contas vivem em TABELAS
-- diferentes — o comprador nesta linha, o vendedor na do anúncio —, e um CHECK
-- só enxerga a própria linha. Ela é recusada no código, na hora de abrir a
-- cobrança, com aviso ao jogador.
--
-- E vale o porquê, porque à primeira vista não há ganho em comprar de si mesmo:
-- o item volta para o mesmo baú e o dinheiro sai da própria conta. O custo não é
-- o item, é a CHAMADA — cada cobrança bate na processadora, que cobra por isso,
-- e vira um jeito barato de sujar a fila e a conciliação de graça.

-- Quem varre as abertas: para expirar, para cancelar quando o comprador cai, e
-- para decidir se o escrow pode soltar o item.
CREATE INDEX rmt_cobranca_abertas ON rmt_cobranca (expira_em) WHERE status = 1;

-- A fila que precisa de gente: dinheiro entrou e o comprador não tem item.
CREATE INDEX rmt_cobranca_sem_item ON rmt_cobranca (paga_em) WHERE status = 5;

-- SOBRE O ESTADO 5, PAGA_SEM_ITEM, que é o que não se pode esconder:
--
-- O caminho existe e a janela curta o torna MAIS provável, não menos: a cobrança
-- expira, o escrow solta o item, o vendedor vende ou usa esse item, e o Pix cai
-- depois. Quanto mais cedo a gente solta, mais tempo sobra para o atraso chegar.
--
-- Aí o dinheiro entrou, o comprador não tem item, e o servidor não pode fingir
-- que entregou. Marcar PAGA com entrega_id nulo seria indistinguível de "ainda
-- não entreguei" e a linha sumiria no meio das normais. Este estado existe para
-- ela NÃO sumir: é dívida com uma pessoa, e o conserto — devolver, compensar,
-- arrumar outro item — é decisão de quem manda, caso a caso, nunca do código.

-- NADA DE CASCADE EM LINHA DE DINHEIRO, e é por isso que as três chaves acima
-- são RESTRICT.
--
-- Com CASCADE no vendedor, apagar a conta levaria os anúncios junto; e como a
-- cobrança aponta para o anúncio, ou o apagamento quebra, ou alguém "conserta"
-- pondo cascade ali também e o REGISTRO DE PAGAMENTO some junto. Registro de
-- dinheiro sobrevive à conta, sempre — é ele que responde "esse Pix foi de quê"
-- seis meses depois, inclusive quando quem pergunta é a processadora.
--
-- Com RESTRICT, conta com histórico de venda deixa de poder ser apagada em
-- silêncio. Isso é recurso e não empecilho: aparece o erro, e uma pessoa decide
-- o que fazer com o histórico em vez de descobrir que ele evaporou.

-- rmt_recebedor: a chave Pix de quem RECEBE.
--
-- Tabela e não coluna no account, espelhando o donate_payer_profile (0010), que
-- é o vizinho exato: um dado de pagamento por conta, preenchido pelo site.
--
-- O CPF do PAGADOR continua lá e não se duplica aqui. São papéis diferentes, e
-- na maioria das vezes nem é a mesma pessoa.
--
-- Este CASCADE fica: é dado pessoal, e sumir com a conta é o certo. O que não
-- pode sumir é o registro do dinheiro, que está nas outras duas.
CREATE TABLE rmt_recebedor (
    account_id    BIGINT PRIMARY KEY REFERENCES account(id) ON DELETE CASCADE,
    chave         TEXT     NOT NULL,
    tipo          SMALLINT NOT NULL,  -- 1=CPF 2=EMAIL 3=TELEFONE 4=ALEATORIA
    verificada_em TIMESTAMPTZ,        -- NULL = não verificada
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
