-- 0115_cobranca_identifier_syncpay — o id DELES.
--
-- DUAS REFERÊNCIAS E NÃO UMA, e a diferença é de dono:
--
--   referencia_externa é NOSSA. Nasce antes da chamada à processadora e é a
--   âncora da idempotência — a confirmação repetida encontra a mesma linha por
--   ela. É o molde do donate_topup_order (0010).
--
--   identifier_syncpay é DELES. Só existe depois de a cobrança nascer lá, e é o
--   que a consulta da transação e o pedido de reembolso exigem: a doc diz
--   "nunca o id interno".
--
-- Juntar as duas numa coluna só quebraria a idempotência, porque a nossa precisa
-- existir ANTES da chamada e a deles só existe DEPOIS. Entre uma e outra há uma
-- chamada de rede que pode falhar.
ALTER TABLE rmt_cobranca
    ADD COLUMN IF NOT EXISTS identifier_syncpay TEXT;

-- Único onde não é nulo: dois registros nossos apontando para a mesma cobrança
-- da processadora significaria pedir dois reembolsos do mesmo dinheiro. Parcial
-- porque o nulo é o estado normal enquanto a chamada não voltou.
CREATE UNIQUE INDEX IF NOT EXISTS rmt_cobranca_identifier
    ON rmt_cobranca (identifier_syncpay) WHERE identifier_syncpay IS NOT NULL;
