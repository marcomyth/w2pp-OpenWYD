ALTER TABLE rmt_recebedor_historico
    DROP COLUMN IF EXISTS documento_antigo_mascarado,
    DROP COLUMN IF EXISTS documento_novo_mascarado;
