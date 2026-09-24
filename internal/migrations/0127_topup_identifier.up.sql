-- 0127_topup_identifier — o id da processadora para a cobrança de DOAÇÃO.
--
-- O BURACO QUE ELE TAPA: até aqui a doação tinha um caminho só para virar crédito —
-- a processadora avisa o site, o site chama o ConfirmTopupOrder. Um aviso perdido era
-- dinheiro cobrado e Rcoin não creditado, e NADA do lado do servidor era capaz de
-- perceber: ele nunca tinha visto o id da processadora, e não se pergunta por um
-- pagamento que não se sabe nomear.
--
-- A venda entre jogadores não tem esse problema desde a 0115, que guarda o
-- identifier da cobrança. Esta coluna é a mesma ideia, do outro lado.
--
-- POR QUE ELE NÃO NASCE AQUI COMO NASCE NO RMT: quem cria a cobrança da doação é o
-- SITE, direto na processadora, com a credencial dele — o servidor não participa.
-- Então o id chega por uma chamada (AttachTopupCharge) logo depois da criação, e não
-- por uma resposta que a gente mesmo pediu.
ALTER TABLE donate_topup_order
    ADD COLUMN IF NOT EXISTS gateway_identifier TEXT;

-- O ÍNDICE ÚNICO É UMA TRAVA DE DINHEIRO, e não arrumação.
--
-- Sem ele, o mesmo id poderia ficar preso a DOIS pedidos pendentes — por bug do site
-- ou por chamada forjada — e a varredura, achando aquele pagamento, creditaria os
-- dois. Uma doação, dois créditos, e o segundo sai do bolso da dona do servidor.
--
-- Parcial porque a esmagadora maioria das linhas não tem id: os pedidos antigos e os
-- que o Attach não alcançou. Nulo não conflita com nulo, e um índice cheio de nulos
-- seria só peso.
CREATE UNIQUE INDEX IF NOT EXISTS donate_topup_order_identifier
    ON donate_topup_order (gateway_identifier)
 WHERE gateway_identifier IS NOT NULL;

-- A fila da varredura: pendente e com id, que é o que dá para consultar.
CREATE INDEX IF NOT EXISTS donate_topup_order_a_conferir
    ON donate_topup_order (created_at DESC)
 WHERE status = 1 AND gateway_identifier IS NOT NULL;
