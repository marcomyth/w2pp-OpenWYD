ALTER TABLE rmt_repasse DROP CONSTRAINT IF EXISTS rmt_repasse_taxa_casa_inteira;
ALTER TABLE rmt_repasse
  DROP COLUMN IF EXISTS taxa_casa_centavos,
  DROP COLUMN IF EXISTS taxa_casa_bps,
  DROP COLUMN IF EXISTS taxa_casa_fixa;
