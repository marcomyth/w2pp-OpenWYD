-- Desfaz a fila de denúncias. Derrubar a tabela descarta o que foi reportado:
-- é relato, não estado, e nada mais lê isso.
DROP INDEX IF EXISTS player_report_expira_idx;
DROP INDEX IF EXISTS player_report_conta_idx;
DROP INDEX IF EXISTS player_report_abertos_idx;
DROP TABLE IF EXISTS player_report;
