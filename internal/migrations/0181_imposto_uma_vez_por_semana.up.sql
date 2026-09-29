-- Quando a guilda dona mudou o imposto da cidade pela última vez.
--
-- A regra passou de uma troca por dia para uma por semana, com a semana virando na
-- segunda-feira 00:00 de Brasília (pedido de 29/09/2026; handler.podeMudarImposto).
-- Antes a data vivia só na memória do tmServer, e cada reinício ou deploy liberava
-- uma troca nova. Aqui ela sobrevive ao reinício.
--
-- NULL é "nunca mudou": a guilda pode mudar já. Ninguém fica preso ao ligar a migração.
ALTER TABLE guild_zone ADD COLUMN IF NOT EXISTS tax_changed_at TIMESTAMPTZ;

-- DONO NOVO NÃO HERDA A ESPERA DO ANTERIOR. A guerra de cidades é no domingo, e quem
-- ganha tem de poder mexer no imposto na hora, mesmo que o dono anterior tenha
-- mudado no sábado. No banco, e não só no Go, porque o dono muda por mais de um
-- caminho (o tmServer, o painel da staff, um UPDATE à mão), e todos passam por aqui.
CREATE OR REPLACE FUNCTION guild_zone_dono_novo_zera_imposto() RETURNS trigger AS $$
BEGIN
    IF NEW.charge_guild IS DISTINCT FROM OLD.charge_guild THEN
        NEW.tax_changed_at := NULL;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS guild_zone_dono_novo_zera_imposto ON guild_zone;
CREATE TRIGGER guild_zone_dono_novo_zera_imposto
    BEFORE UPDATE ON guild_zone
    FOR EACH ROW EXECUTE FUNCTION guild_zone_dono_novo_zera_imposto();
