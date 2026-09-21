-- Desfaz o 0079. As colunas saem com os dados dentro: o recado, o status de cada
-- membro e o teto que alguém tenha ajustado não voltam.
ALTER TABLE guild_member DROP CONSTRAINT IF EXISTS guild_member_status_len_check;
ALTER TABLE guild_member DROP COLUMN IF EXISTS last_seen;
ALTER TABLE guild_member DROP COLUMN IF EXISTS status;

ALTER TABLE guild DROP CONSTRAINT IF EXISTS guild_member_cap_check;
ALTER TABLE guild DROP CONSTRAINT IF EXISTS guild_notice_len_check;
ALTER TABLE guild DROP COLUMN IF EXISTS member_cap;
ALTER TABLE guild DROP COLUMN IF EXISTS notice_by;
ALTER TABLE guild DROP COLUMN IF EXISTS notice_at;
ALTER TABLE guild DROP COLUMN IF EXISTS notice;
