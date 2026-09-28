-- 0124_repasse_ao_vendedor — a quem pertence o dinheiro que entrou.
--
-- O QUE NÃO EXISTIA, e é o buraco que esta tabela fecha: quando uma venda em dinheiro
-- real se concluía, o comprador recebia o item e o dinheiro ficava na conta de quem
-- administra o servidor SEM NENHUM REGISTRO DE A QUEM ELE PERTENCE. Não era um repasse
-- atrasado — era a ausência de qualquer linha dizendo que havia dívida.
--
-- A LINHA NASCE NA MESMA TRANSAÇÃO que marca a cobrança PAGA. Fora dela abre a janela
-- em que o item sai, o dinheiro entra, e a dívida com o vendedor não fica escrita: uma
-- queda ali deixaria a venda completa e o vendedor invisível, e ninguém saberia
-- procurar por ele.
--
-- E NASCE SÓ NA VENDA CONCLUÍDA. Nada em PAGA_SEM_ITEM, nada no valor divergente —
-- nesses dois o dinheiro vai VOLTAR para o comprador, e o vendedor não tem nada a
-- receber. Uma linha de dívida que não existe é pior do que a ausência dela: ela
-- apareceria na fila, alguém tentaria pagar, e o dinheiro sairia duas vezes do mesmo
-- lugar.
CREATE TABLE rmt_repasse (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,

    -- UMA COBRANÇA, UM REPASSE. O único é o que faz a idempotência valer: a
    -- confirmação repetida encontra a linha que já existe em vez de criar uma segunda
    -- dívida pela mesma venda. Vale mais aqui do que em qualquer outro lugar deste
    -- sistema, porque pagar duas vezes não se desfaz.
    cobranca_id BIGINT NOT NULL UNIQUE REFERENCES rmt_cobranca(id) ON DELETE RESTRICT,
    -- RESTRICT, como todas as linhas de dinheiro (0105): registro de pagamento
    -- sobrevive à conta. É ele que responde "esse dinheiro era de quem" seis meses
    -- depois, inclusive quando quem pergunta é a processadora.
    vendedor_conta BIGINT NOT NULL REFERENCES account(id) ON DELETE RESTRICT,

    -- O VALOR DA VENDA, copiado no instante em que a dívida nasce.
    --
    -- Copiado e não lido do anúncio por JOIN: o preço do anúncio é de hoje, e a dívida
    -- é do dia da venda. Se o vendedor reanunciar mais caro, o JOIN passaria a dever
    -- mais do que foi vendido — e ninguém notaria, porque o número continuaria
    -- parecendo certo.
    valor_centavos BIGINT NOT NULL CHECK (valor_centavos > 0),

    -- 1=PENDENTE 2=ENVIADO 3=PAGO 4=RECUSADO 5=INCERTO
    --
    -- O INCERTO É O ESTADO QUE NÃO PODE FALTAR, e é o menos óbvio dos cinco.
    --
    -- A ponte responde 502/incerto quando a chamada saiu e a resposta não voltou:
    -- PODE TER PAGO. A referência dela trava para sempre, e reenviar é a única coisa
    -- que não se pode fazer — pagar duas vezes não se desfaz. Sem um estado próprio,
    -- ele ficaria PENDENTE e a varredura seguinte mandaria de novo.
    --
    -- E ele é diferente de ENVIADO: o enviado foi ACEITO, tem o id do saque, e vira
    -- PAGO quando o aviso de saque chegar. O incerto não tem id, não vira nada
    -- sozinho, e espera uma pessoa olhar o painel da processadora.
    --
    -- Não há consulta de saque para desempatar: a V2 exclui saques, e a V1 não foi
    -- medida para eles. Então o incerto é resolvido por gente, e é por isso que ele
    -- tem fila.
    status SMALLINT NOT NULL DEFAULT 1,

    -- O id do saque DELES, que chega na resposta do envio. É por ele que o aviso de
    -- CASHOUT encontra este repasse; sem ele a taxa do saque fica sem dono.
    identifier_saque TEXT,

    -- O que a gente PEDIU e o que de fato CHEGOU na conta do vendedor. A diferença é a
    -- taxa do saque, e ela só se sabe pelo aviso de CASHOUT, depois.
    --
    -- Duas colunas e não uma com a taxa: guardar a diferença calculada esconde qual das
    -- duas pontas mudou no dia em que elas discordarem do esperado.
    enviado_centavos BIGINT,
    chegou_centavos  BIGINT,

    -- DE QUEM VEIO A RECUSA, que é a distinção que muda quem tem de agir.
    --
    -- Nulo quer dizer que quem recusou foi a PRÓPRIA PONTE — teto por repasse, teto
    -- diário, ou a trava do saque desligada. Nenhuma dessas é culpa do vendedor, e
    -- tratá-las como "sua chave está errada" mandaria a pessoa mexer no que estava
    -- certo. Com número, a recusa é da processadora.
    --
    -- Isso importa AGORA e não em teoria: enquanto a trava do saque estiver desligada
    -- na ponte, TODO repasse volta recusado com este campo nulo.
    recusa_http INTEGER,

    -- O motivo CRU da recusa, como a processadora deu, sem resumir. "Chave não
    -- existe", "documento não confere com a chave" e "saldo insuficiente" pedem ações
    -- completamente diferentes — as duas primeiras o vendedor conserta, a terceira é
    -- de quem administra — e viram todas "falhou" se alguém as traduzir cedo demais.
    recusa_codigo TEXT,
    recusa_texto  TEXT,

    criado_em  TIMESTAMPTZ NOT NULL DEFAULT now(),
    enviado_em TIMESTAMPTZ,
    pago_em    TIMESTAMPTZ,
    -- Quando uma pessoa tratou a recusa. Nulo = está na fila.
    resolvido_em  TIMESTAMPTZ,
    resolvido_por TEXT
);

-- A fila de quem tem dinheiro a receber, mais antigo primeiro. É esta consulta que
-- responde "quem está esperando", que é a pergunta que ninguém conseguia fazer antes.
CREATE INDEX rmt_repasse_a_pagar ON rmt_repasse (criado_em) WHERE status = 1;

-- A fila da staff: o que travou e precisa de gente.
CREATE INDEX rmt_repasse_recusado ON rmt_repasse (criado_em)
    WHERE status = 4 AND resolvido_em IS NULL;

-- A outra fila de gente, e a mais urgente das duas: o que PODE ter sido pago e
-- ninguém sabe. Cada linha aqui é um vendedor que talvez já tenha o dinheiro e talvez
-- não, e a única forma de descobrir é alguém olhar o painel da processadora.
CREATE INDEX rmt_repasse_incerto ON rmt_repasse (criado_em)
    WHERE status = 5 AND resolvido_em IS NULL;

-- O caminho do aviso de CASHOUT: do identifier deles para o nosso repasse.
CREATE UNIQUE INDEX rmt_repasse_identifier ON rmt_repasse (identifier_saque)
    WHERE identifier_saque IS NOT NULL;
