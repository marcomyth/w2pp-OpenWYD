-- A volta derruba as duas colunas e a fila do estado 6.
--
-- Ela só serve ANTES de haver repasse segurado: descer com linhas em 6 as deixaria num
-- estado que o código de trás não conhece, e a fila de pagar, que pede status = 1,
-- nunca as encontraria. Ficariam paradas sem ninguém as ver.
DROP INDEX IF EXISTS rmt_repasse_sem_taxa;
ALTER TABLE rmt_repasse DROP COLUMN IF EXISTS bruto_centavos;
ALTER TABLE rmt_cobranca DROP COLUMN IF EXISTS taxa_centavos;

-- O comentário do status vai a NULO, e não a um texto antigo: a 0124 não escreveu
-- comentário nenhum nesta coluna, e recolocar uma frase "anterior" inventaria um
-- registro que nunca existiu.
COMMENT ON COLUMN rmt_repasse.status IS NULL;
