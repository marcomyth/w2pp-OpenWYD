-- A volta só serve ANTES de existir linha de apagar: com uma delas gravada, o NOT NULL
-- não pode ser restaurado, e o ALTER falha. É o certo — descer aqui com histórico de
-- apagar exigiria apagar esse histórico, e apagar histórico para caber num esquema
-- antigo é perder o registro que ele existe para guardar.
ALTER TABLE rmt_recebedor_historico ALTER COLUMN chave_nova_mascarada SET NOT NULL;
ALTER TABLE rmt_recebedor_historico ALTER COLUMN tipo_novo SET NOT NULL;
