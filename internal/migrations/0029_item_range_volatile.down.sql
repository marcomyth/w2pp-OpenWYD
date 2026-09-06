-- Desfaz as duas colunas. Descartar é seguro: sem elas o jogo volta a ler
-- EF_RANGE e EF_VOLATILE só do ItemList.csv, que é o comportamento anterior.
ALTER TABLE item_stat
    DROP COLUMN IF EXISTS ef_volatile,
    DROP COLUMN IF EXISTS ef_range;
