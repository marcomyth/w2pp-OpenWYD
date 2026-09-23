-- 0114_cobranca_reembolso — o destino do dinheiro que chegou tarde.
--
-- O DESTINO DO DINHEIRO QUE CHEGOU TARDE.
--
-- Até aqui o estado PAGA_SEM_ITEM era dívida com uma pessoa e mais nada — a linha
-- ficava numa fila para alguém olhar. A decisão da Hanna deu destino a ela:
-- reembolso automático, sem item.
--
-- O ESTADO DO PEDIDO É COLUNA e não é deduzido do estado da cobrança, porque o
-- reembolso tem vida própria e demorada: a SyncPay reserva o valor na hora e a
-- ANÁLISE LEVA ATÉ DOIS DIAS ÚTEIS (medido na tela deles, 23/09/2026 — não está
-- na doc da API). Reprovado ou cancelado, o valor volta; aprovado, a taxa de
-- R$ 1,00 é cobrada e as taxas da venda original não são estornadas.
--
-- 1=PEDIDO 2=APROVADO 3=CONCLUIDO 4=RECUSADO
ALTER TABLE rmt_cobranca
    ADD COLUMN IF NOT EXISTS reembolso_status SMALLINT;

-- QUANDO o pedido foi feito, e não quanto falta: é a partir desta data que a
-- página do comprador conta os "até 2 dias úteis". Uma contagem calculada e
-- guardada já nasceria errada.
ALTER TABLE rmt_cobranca
    ADD COLUMN IF NOT EXISTS reembolso_pedido_em TIMESTAMPTZ;

-- O que a processadora respondeu quando recusou, guardado como veio. Um reembolso
-- RECUSADO para na tela da staff e NÃO é tentado de novo em laço — e quem for
-- olhar precisa do código deles, não da nossa interpretação dele.
ALTER TABLE rmt_cobranca
    ADD COLUMN IF NOT EXISTS reembolso_erro TEXT;

-- A fila que precisa de gente: dinheiro entrou, item não saiu, e o reembolso
-- ainda não concluiu.
CREATE INDEX IF NOT EXISTS rmt_cobranca_reembolso_aberto
    ON rmt_cobranca (reembolso_pedido_em)
 WHERE reembolso_status IS NOT NULL AND reembolso_status < 3;
