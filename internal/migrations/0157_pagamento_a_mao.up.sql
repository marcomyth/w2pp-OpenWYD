-- 0153_pagamento_a_mao — quem pagou o vendedor à mão, e o que ela disse que fez.
--
-- POR QUE ISTO EXISTE. O saque automático saiu (decisão da Hanna, 25/09/2026): quem
-- paga o vendedor agora é a staff, fora do sistema, e marca a linha como paga no painel.
-- Essa é a única escrita de dinheiro deste servidor que NÃO tem comprovante nosso
-- atrás dela — não houve chamada a processadora nenhuma, só uma pessoa clicando.
--
-- Então o que sobra para reconstruir a história é o que está aqui. A observação diz ONDE
-- o dinheiro foi pago e como achar o comprovante; a coluna de quem diz o nome de quem
-- clicou. As duas são gravadas na MESMA transação da mudança de estado.
--
-- E A AUDITORIA, ENTÃO? Ela também é gravada, na mesma transação. Estas colunas não são
-- cópia dela: a auditoria é a trilha da staff, lida por ação e por ator, e sobrevive à
-- linha; estas duas ficam NA LINHA, e são o que a tela do repasse mostra sem precisar
-- cruzar tabela. Numa disputa sobre um repasse específico, a pergunta é "esta linha, o
-- que foi feito com ela" — e é caro responder isso varrendo log de auditoria.
--
-- NADA DE NOT NULL. Toda linha paga ANTES desta migração foi paga pela rota automática,
-- e para ela estas colunas são nulas com um significado próprio: "não foi à mão".
-- Inventar um valor de enchimento para o passado apagaria justamente essa diferença.
ALTER TABLE rmt_repasse
    ADD COLUMN IF NOT EXISTS pago_a_mao_nota TEXT,
    ADD COLUMN IF NOT EXISTS pago_a_mao_por  TEXT;

-- O teto da observação acompanha o do código (store.MaxNotaDoPagamento = 500). A trava
-- no banco existe porque o texto vem de um formulário: sem ela, um POST montado à mão
-- guardaria um megabyte numa coluna que a tela lê em toda visita à fila.
ALTER TABLE rmt_repasse
    ADD CONSTRAINT rmt_repasse_pago_a_mao_nota_tamanho
    CHECK (pago_a_mao_nota IS NULL OR char_length(pago_a_mao_nota) <= 500);

-- PAGO À MÃO EXIGE AS DUAS, e é o banco que garante: uma nota sem autor não responde
-- "quem", e um autor sem nota não responde "onde". O estado 3 é o PAGO (0124).
--
-- A regra NÃO é "todo pago tem as duas", de propósito: o pago pela rota automática não
-- tem nenhuma, e é isso que o distingue.
ALTER TABLE rmt_repasse
    ADD CONSTRAINT rmt_repasse_pago_a_mao_completo
    CHECK ((pago_a_mao_nota IS NULL) = (pago_a_mao_por IS NULL));
