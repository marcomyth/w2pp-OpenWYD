-- Volta ao CHECK de dois tipos da 0034.
--
-- As linhas dos quatro canais novos têm de sair antes, ou o CHECK antigo não
-- entra. Não há para onde movê-las: reescrever uma fala de guilda como
-- 'publico' gravaria um alcance que não é o dela, e é o alcance que este
-- registro existe para guardar. Descer esta migração APAGA conversa, e é por
-- isso que ela não deve descer num servidor vivo.

DELETE FROM chat_log WHERE tipo IN ('guilda', 'grupo', 'reino', 'cidadao');

ALTER TABLE chat_log DROP CONSTRAINT IF EXISTS chat_log_tipo_check;

ALTER TABLE chat_log ADD CONSTRAINT chat_log_tipo_check
    CHECK (tipo IN ('publico', 'sussurro'));
