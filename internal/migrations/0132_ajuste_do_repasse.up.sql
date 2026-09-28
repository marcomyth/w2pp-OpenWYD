-- O AJUSTE DE VALOR DE UM REPASSE RECUSADO, com o número antigo guardado na linha.
--
-- Nasce de um caso real: a venda de R$ 1,00 de 25/09/2026 gerou um repasse de R$ 1,00
-- CHEIO, porque o desconto da taxa (0131) ainda não estava no ar. A processadora ficou
-- com R$ 0,80 e o vendedor receberia R$ 1,00 — a casa pagaria a diferença. A ordem da
-- Hanna foi reenviar o líquido, R$ 0,20, e não o cheio.
--
-- O VALOR ANTIGO FICA NA PRÓPRIA LINHA, e não só na tabela de auditoria. Auditoria é
-- escrita DEPOIS da mudança neste sistema, e o próprio código do painel diz que a
-- mudança pode acontecer e o registro não. Para um número de dinheiro que foi
-- sobrescrito, isso não serve: sem o valor antigo em algum lugar que a mesma transação
-- garanta, ninguém consegue responder "quanto era antes" quando a auditoria falhar. As
-- duas escritas convivem — a auditoria conta a história, a linha guarda a prova.
ALTER TABLE rmt_repasse ADD COLUMN ajuste_de_centavos BIGINT;
ALTER TABLE rmt_repasse ADD COLUMN ajuste_nota TEXT;
ALTER TABLE rmt_repasse ADD COLUMN ajustado_por TEXT;
ALTER TABLE rmt_repasse ADD COLUMN ajustado_em TIMESTAMPTZ;

COMMENT ON COLUMN rmt_repasse.ajuste_de_centavos IS
  'Quanto a linha valia ANTES do ajuste da staff. NULO = nunca foi ajustada.';
COMMENT ON COLUMN rmt_repasse.ajuste_nota IS
  'Por que o valor mudou. Obrigatoria na acao: numero de dinheiro sem motivo nao se explica depois.';
