-- 0125_tentativa_de_repasse — cada envio, com a referência que ele usou.
--
-- POR QUE UMA DÍVIDA TEM MAIS DE UMA REFERÊNCIA, que é a pergunta que alguém vai fazer
-- ao ler isto daqui a seis meses:
--
-- A ponte é idempotente pela referência, e a trava dela é PARA SEMPRE. Quando ela
-- responde "incerto" — a chamada saiu e a resposta não voltou —, aquela referência fica
-- queimada: reenviar com ela devolve o resultado da primeira vez, que ninguém sabe qual
-- foi. E não há consulta de saque para descobrir.
--
-- Então, quando uma pessoa vai ao painel da processadora, vê que NÃO pagou, e manda
-- tentar de novo, a segunda tentativa precisa de uma referência NOVA. Uma dívida, duas
-- referências — e sem esta tabela ninguém saberia por quê, nem qual delas foi a que
-- ficou pendurada.
--
-- E ela responde a pergunta que uma disputa faz: "quantas vezes vocês tentaram me
-- pagar, quando, e o que deu em cada uma".
CREATE TABLE rmt_repasse_tentativa (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    repasse_id BIGINT NOT NULL REFERENCES rmt_repasse(id) ON DELETE RESTRICT,

    -- O número da tentativa, 1, 2, 3. É o que entra na referência, e é o que faz duas
    -- tentativas da MESMA dívida terem referências diferentes.
    tentativa SMALLINT NOT NULL CHECK (tentativa > 0),

    -- A referência que FOI ao fio nesta tentativa, gravada ANTES da chamada.
    --
    -- Antes e não depois: se a resposta se perder, é por ela que se descobre o que foi
    -- mandado. Uma referência gravada só no sucesso deixaria o incerto — o caso em que
    -- ela mais importa — sem registro nenhum.
    referencia TEXT NOT NULL UNIQUE,

    -- O que voltou. Os mesmos estados do repasse, e nulo enquanto a chamada está no ar.
    resultado    SMALLINT,
    recusa_http  INTEGER,
    recusa_codigo TEXT,
    recusa_texto  TEXT,
    -- O id do saque, quando a ponte aceitou.
    identifier_saque TEXT,

    -- QUEM LIBEROU esta tentativa. Nulo na primeira, que é automática; preenchido a
    -- partir da segunda, que só existe porque uma pessoa foi olhar o painel e disse que
    -- não tinha pago.
    --
    -- É o que liga a tentativa nova à decisão que a autorizou. Sem isso, um segundo
    -- pagamento para o mesmo vendedor pareceria um bug em vez de uma decisão.
    liberado_por TEXT,

    criado_em    TIMESTAMPTZ NOT NULL DEFAULT now(),
    respondido_em TIMESTAMPTZ
);

-- Uma tentativa por número, por dívida. No banco, porque "não mandar duas vezes a
-- mesma tentativa" é invariante de dinheiro, e invariante que depende de alguém lembrar
-- não é invariante.
CREATE UNIQUE INDEX rmt_repasse_tentativa_unica
    ON rmt_repasse_tentativa (repasse_id, tentativa);

-- A leitura de uma disputa: o histórico desta dívida, do começo.
CREATE INDEX rmt_repasse_tentativa_por_repasse
    ON rmt_repasse_tentativa (repasse_id, tentativa);
