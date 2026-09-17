-- 0073_teto_de_xp_por_rodada — o teto de XP por rodada do Mortal.
--
-- Decidido em 17/09/2026. A cada rodada de 600 s do relógio das arenas, toda XP
-- que o Mortal recebe soma no máximo o teto da faixa do nível dele no momento do
-- ganho; o troféu, no máximo a metade (tmserver/internal/handler/tetorodada.go).
-- Com o dobro ligado vale a coluna _double. Zero numa faixa = sem teto naquela
-- faixa, o jeito de desligar sem deploy.
--
-- As faixas são pelo nível GUARDADO: 1-99, 100-199, 200-299, 300-349 e 350-398.
-- Os padrões são P/6, com P = XP exigida da faixa / horas-alvo e a média do dobro
-- (53 h de 168 por semana) tirada: quem enche o teto toda rodada faz as horas do
-- plano. Mora na linha que o tmServer relê ao vivo, para a equipe mudar pelo
-- painel de eventos.

ALTER TABLE world_event_config
    ADD COLUMN round_xp_cap_99         BIGINT NOT NULL DEFAULT 604951  CHECK (round_xp_cap_99 >= 0),
    ADD COLUMN round_xp_cap_199        BIGINT NOT NULL DEFAULT 2669386 CHECK (round_xp_cap_199 >= 0),
    ADD COLUMN round_xp_cap_299        BIGINT NOT NULL DEFAULT 500454  CHECK (round_xp_cap_299 >= 0),
    ADD COLUMN round_xp_cap_349        BIGINT NOT NULL DEFAULT 1263881 CHECK (round_xp_cap_349 >= 0),
    ADD COLUMN round_xp_cap_398        BIGINT NOT NULL DEFAULT 1263881 CHECK (round_xp_cap_398 >= 0),
    ADD COLUMN round_xp_cap_double_99  BIGINT NOT NULL DEFAULT 1209902 CHECK (round_xp_cap_double_99 >= 0),
    ADD COLUMN round_xp_cap_double_199 BIGINT NOT NULL DEFAULT 5338772 CHECK (round_xp_cap_double_199 >= 0),
    ADD COLUMN round_xp_cap_double_299 BIGINT NOT NULL DEFAULT 1000908 CHECK (round_xp_cap_double_299 >= 0),
    ADD COLUMN round_xp_cap_double_349 BIGINT NOT NULL DEFAULT 2527761 CHECK (round_xp_cap_double_349 >= 0),
    ADD COLUMN round_xp_cap_double_398 BIGINT NOT NULL DEFAULT 2527761 CHECK (round_xp_cap_double_398 >= 0);
