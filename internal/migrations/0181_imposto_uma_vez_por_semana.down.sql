DROP TRIGGER IF EXISTS guild_zone_dono_novo_zera_imposto ON guild_zone;
DROP FUNCTION IF EXISTS guild_zone_dono_novo_zera_imposto();
ALTER TABLE guild_zone DROP COLUMN IF EXISTS tax_changed_at;
