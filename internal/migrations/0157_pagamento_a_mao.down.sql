-- A volta tira as travas antes das colunas, porque uma restrição não sobrevive à coluna
-- que ela olha.
--
-- ELA APAGA QUEM PAGOU E ONDE, e isso é perda de verdade: a auditoria guarda a mesma
-- informação por ação, mas a linha do repasse deixa de responder sozinha. Voltar esta
-- migração num banco que já tem pagamento à mão só se faz sabendo disso.
ALTER TABLE rmt_repasse
    DROP CONSTRAINT IF EXISTS rmt_repasse_pago_a_mao_completo;
ALTER TABLE rmt_repasse
    DROP CONSTRAINT IF EXISTS rmt_repasse_pago_a_mao_nota_tamanho;
ALTER TABLE rmt_repasse
    DROP COLUMN IF EXISTS pago_a_mao_por,
    DROP COLUMN IF EXISTS pago_a_mao_nota;
