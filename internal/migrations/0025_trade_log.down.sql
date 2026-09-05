-- Undo the trade log. Dropping it discards the recorded trades: they are
-- evidence, not state, and nothing else reads them.
DROP INDEX IF EXISTS trade_log_ocorrido_idx;
DROP INDEX IF EXISTS trade_log_char_b_idx;
DROP INDEX IF EXISTS trade_log_char_a_idx;
DROP TABLE IF EXISTS trade_log;
