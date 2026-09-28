-- 0162_taxa_da_casa_no_repasse — guardar a REGRA que valeu em cada venda.
--
-- A taxa da casa é 5% até R$ 100,00 e 6,99% acima, com R$ 0,80 fixos (taxa_rmt.go).
-- Estes três números guardam o que valeu NAQUELA venda, e não o que vale hoje.
--
-- POR QUE GRAVAR A REGRA E NÃO SÓ O VALOR. No dia em que a Hanna mudar a taxa, toda
-- venda antiga passaria a "não bater" com a tabela nova — sem ninguém ter mexido em
-- nada. Quem fosse conferir a contabilidade de um mês atrás encontraria números que a
-- fórmula de hoje não reproduz, e a conclusão natural seria "o sistema errou".
--
-- Guardar os pontos-base e o fixo deixa o passado continuar fechando sozinho: cada linha
-- carrega a conta que a produziu.
--
-- NULO É "ANTES DA TAXA DA CASA", e não zero. As vendas que já estão no banco
-- aconteceram quando a casa não cobrava nada, e escrever zero nelas diria que a regra
-- existia e deu zero — que é diferente, e faria uma média por faixa mentir.
ALTER TABLE rmt_repasse
  ADD COLUMN taxa_casa_centavos BIGINT   CHECK (taxa_casa_centavos >= 0),
  ADD COLUMN taxa_casa_bps      SMALLINT CHECK (taxa_casa_bps BETWEEN 0 AND 10000),
  ADD COLUMN taxa_casa_fixa     BIGINT   CHECK (taxa_casa_fixa >= 0);

-- OS TRÊS ANDAM JUNTOS: ou a venda tem a regra inteira, ou não tem nenhuma. Dois de três
-- é uma linha que ninguém consegue recalcular, e é o estado em que um bug de escrita
-- deixaria o banco sem nada reclamar.
ALTER TABLE rmt_repasse
  ADD CONSTRAINT rmt_repasse_taxa_casa_inteira CHECK (
    (taxa_casa_centavos IS NULL AND taxa_casa_bps IS NULL AND taxa_casa_fixa IS NULL)
 OR (taxa_casa_centavos IS NOT NULL AND taxa_casa_bps IS NOT NULL AND taxa_casa_fixa IS NOT NULL));

COMMENT ON COLUMN rmt_repasse.taxa_casa_centavos IS
  'O que a casa ficou desta venda, em centavos. NULO = venda anterior a taxa da casa.';
COMMENT ON COLUMN rmt_repasse.taxa_casa_bps IS
  'A faixa percentual que valeu, em pontos-base (500 = 5,00%, 699 = 6,99%).';
COMMENT ON COLUMN rmt_repasse.taxa_casa_fixa IS
  'A parte fixa que valeu, em centavos (80 = R$ 0,80).';
