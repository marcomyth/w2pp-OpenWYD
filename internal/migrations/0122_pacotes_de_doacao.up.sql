-- 0122_pacotes_de_doacao — o que cada pacote de doação vende, no SERVIDOR.
--
-- POR QUE A TABELA MORA AQUI E NÃO NO SITE, que é onde a vitrine está: quem entrega
-- é este lado, e quem entrega tem de saber o que prometeu. Hoje o `CreateTopupOrder`
-- recebe os créditos e o valor JÁ DECIDIDOS pelo pedido, e credita o que vier — então
-- o servidor não tem como saber se aquele número é o do pacote que a pessoa comprou.
--
-- Com a tabela, o pedido passa a dizer só QUAL pacote, e o preço e os créditos que
-- valem são os DAQUI. Os do pedido são conferidos contra estes e, se divergirem, o
-- pedido é recusado. A requisição vem do BFF do site, que é nosso, e ainda assim: o
-- lado que entrega confere, porque conferir é barato e destrocar item entregue não é.
CREATE TABLE donate_pacote (
    -- O id que o SITE usa, em texto, e não um número nosso. Ele viaja no pedido e
    -- aparece no log dos dois lados; um id compartilhado é o que permite alguém ler
    -- "apoiador-supremo" numa investigação em vez de procurar a que número ele
    -- correspondia naquele dia.
    id             TEXT PRIMARY KEY,
    -- O que o pacote dá de moeda da loja.
    credits        INTEGER NOT NULL CHECK (credits >= 0),
    -- Quanto ele custa, em centavos. Nunca em float: dinheiro em ponto flutuante é
    -- como o centavo some.
    amount_cents   BIGINT  NOT NULL CHECK (amount_cents > 0),
    -- SÓ STAFF: o pacote existe, o site o esconde, e o SERVIDOR também o recusa para
    -- quem não é staff. Esconder na tela não é trava — a tela é uma das portas, e
    -- quem chama a RPC direto passaria por ela.
    --
    -- É o que segura o pacote de teste de R$ 1,00, que existe para a dona do servidor
    -- testar pagamento de verdade sem cobrar R$ 50 de si mesma.
    so_staff       BOOLEAN NOT NULL DEFAULT FALSE,
    -- Desligar um pacote NÃO o apaga. Pedido antigo que ainda não confirmou tem de
    -- continuar achando a linha dele, senão a pessoa paga e o servidor não sabe o que
    -- prometeu. Apagar linha de coisa vendida é o mesmo erro que CASCADE em linha de
    -- dinheiro.
    ativo          BOOLEAN NOT NULL DEFAULT TRUE,
    -- Para a tela da staff e para a investigação: quando este pacote passou a existir.
    criado_em      TIMESTAMPTZ NOT NULL DEFAULT now(),
    atualizado_em  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- donate_pacote_item: os BRINDES, um por linha.
--
-- A forma é a do payload da `delivery_queue` de propósito — índice mais os três pares
-- de efeito —, porque é exatamente isso que vai ser enfileirado. Uma forma diferente
-- aqui obrigaria uma tradução no meio, e tradução no meio de entrega de item é onde
-- um efeito se perde sem ninguém ver.
--
-- NÃO HÁ COLUNA DE QUANTIDADE, e isso não é esquecimento: no jogo a quantidade é o
-- efeito EF_AMOUNT, e uma coluna separada criaria duas fontes para o mesmo número.
-- Mesma coisa para a duração, que é EF_WDAY/HOUR/MIN.
CREATE TABLE donate_pacote_item (
    id         BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    pacote_id  TEXT     NOT NULL REFERENCES donate_pacote(id) ON DELETE RESTRICT,
    item_index INTEGER  NOT NULL CHECK (item_index > 0),
    eff1       SMALLINT NOT NULL DEFAULT 0,
    effv1      SMALLINT NOT NULL DEFAULT 0,
    eff2       SMALLINT NOT NULL DEFAULT 0,
    effv2      SMALLINT NOT NULL DEFAULT 0,
    eff3       SMALLINT NOT NULL DEFAULT 0,
    effv3      SMALLINT NOT NULL DEFAULT 0,
    -- A ordem em que os itens entram na caixa postal. Importa pouco e custa nada, e
    -- evita que a mesma compra entregue em ordens diferentes em duas rodadas.
    ordem      SMALLINT NOT NULL DEFAULT 0
);

CREATE INDEX donate_pacote_item_pacote_idx ON donate_pacote_item (pacote_id, ordem, id);

-- A DURAÇÃO DOS BRINDES VAI NÃO INICIADA, e isso não é coluna: é a AUSÊNCIA de
-- `expires_at` no payload, que fica zero.
--
-- A diferença importa para quem recebe. Com data absoluta, uma montaria de 15 dias
-- comprada numa sexta começa a gastar prazo na hora, mesmo que a pessoa só entre no
-- domingo — e se ela não entrar por duas semanas, recebe um item vencido. Com o efeito
-- EF_WDAY/HOUR/MIN, a contagem começa no PRIMEIRO USO.
--
-- Quem paga por quinze dias tem de receber quinze dias de uso, e não quinze dias de
-- calendário a partir de um instante que ela não escolheu.

-- O pedido passa a dizer QUAL pacote. Nulo em todo pedido anterior a esta migração,
-- onde nulo quer dizer "veio antes de existir pacote", e não "pacote desconhecido".
ALTER TABLE donate_topup_order
    ADD COLUMN IF NOT EXISTS pacote_id TEXT REFERENCES donate_pacote(id) ON DELETE RESTRICT;
