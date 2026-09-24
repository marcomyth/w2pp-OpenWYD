-- 0118_cobranca_valor_divergente — o dinheiro que entrou com o valor errado.
--
-- A processadora diz ter recebido um valor diferente do que a cobrança pedia.
-- Pagar menos e receber o item seria comprar com desconto de si mesmo; pagar mais
-- e receber sem troco seria o contrário. Os dois pedem uma pessoa, e nenhum pede
-- decisão automática.
--
-- O QUE ISTO CONSERTA: antes, esse caso não gravava NADA. O dinheiro tinha entrado,
-- a linha continuava aberta, e a varredura do prazo a vencia e soltava o item — o
-- pagamento sumia do nosso registro. Alguém pagou e não há linha que diga isso.
--
-- COLUNA E NÃO STATUS NOVO, e a escolha é de RISCO e não de gosto.
--
-- Um status 6 seria mais bonito e obrigaria a mexer em tudo que pergunta "há
-- dinheiro em jogo?": o encerramento da barraca, a reconciliação, a faxina dos
-- cadeados, os dois índices de uma-aberta, a leitura do comprador. Esquecer UM
-- desses lugares solta o item de alguém que pagou. São seis chances de errar num
-- caminho onde errar custa o item de um jogador.
--
-- Mantendo `status = ABERTA`, tudo isso continua certo sem tocar em nada, porque a
-- cobrança de fato AINDA está em aberto — ela não se resolveu. Só duas coisas
-- mudam, e as duas são explícitas: a varredura do prazo pula a divergente (senão
-- ela venceria e soltaria o item), e a página do comprador não mostra código de
-- pagamento para ela.
ALTER TABLE rmt_cobranca
    ADD COLUMN IF NOT EXISTS valor_divergente_centavos BIGINT;

-- A fila da staff: dinheiro entrou com o valor errado e ninguém decidiu o que
-- fazer. Parcial porque o caso é raro por construção.
CREATE INDEX IF NOT EXISTS rmt_cobranca_valor_divergente
    ON rmt_cobranca (id) WHERE valor_divergente_centavos IS NOT NULL;
