-- Volta ao vocabulário de quatro estados. Repare que ele NÃO apaga as linhas em
-- INCERTO (5): desfazer o CHECK não desfaz o que aconteceu com o dinheiro, e apagar
-- o estado de uma devolução que talvez tenha sido criada seria perder justamente a
-- informação que alguém vai precisar.
ALTER TABLE rmt_cobranca
    DROP CONSTRAINT IF EXISTS rmt_cobranca_reembolso_status_conhecido;
