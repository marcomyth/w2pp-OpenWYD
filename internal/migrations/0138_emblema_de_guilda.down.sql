ALTER TABLE guild DROP CONSTRAINT IF EXISTS guild_emblema_630_bytes;
ALTER TABLE guild
  DROP COLUMN emblema,
  DROP COLUMN emblema_trocado_em;
