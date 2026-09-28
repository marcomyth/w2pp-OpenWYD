DROP INDEX IF EXISTS account_dono_epoca_idx;
ALTER TABLE account
  DROP COLUMN dono_epoca,
  DROP COLUMN dono_desde,
  DROP COLUMN dono_batimento;
