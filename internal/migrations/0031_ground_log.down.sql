-- Desfaz o registro do chão. Derrubar descarta o que foi registrado: é prova,
-- não estado, e nada mais lê isso.
DROP INDEX IF EXISTS ground_log_expira_idx;
DROP INDEX IF EXISTS ground_log_ocorrido_idx;
DROP INDEX IF EXISTS ground_log_chao_idx;
DROP INDEX IF EXISTS ground_log_char_idx;
DROP TABLE IF EXISTS ground_log;
