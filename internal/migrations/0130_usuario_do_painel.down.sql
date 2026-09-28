-- A VOLTA APAGA AS AÇÕES FEITAS POR USUÁRIO DE PAINEL, e não há como não apagar:
-- a coluna que as atribui deixa de existir, e o NOT NULL de volta exige que toda
-- linha tenha conta de jogo. Então a ordem aqui é deliberada — primeiro somem as
-- linhas sem conta de jogo, depois a coluna, depois a tabela.
--
-- Quem rodar isto em produção perde auditoria. Está escrito para a decisão ser
-- consciente, não para ser confortável.
ALTER TABLE admin_audit_log DROP CONSTRAINT IF EXISTS admin_audit_log_um_ator;
DROP INDEX IF EXISTS admin_audit_log_ator_painel_idx;
DELETE FROM admin_audit_log WHERE actor_account_id IS NULL;
ALTER TABLE admin_audit_log DROP COLUMN IF EXISTS actor_painel_usuario_id;
ALTER TABLE admin_audit_log ALTER COLUMN actor_account_id SET NOT NULL;
DROP TABLE IF EXISTS painel_usuario;
