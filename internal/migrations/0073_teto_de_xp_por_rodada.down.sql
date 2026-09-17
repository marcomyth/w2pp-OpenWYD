ALTER TABLE world_event_config
    DROP COLUMN IF EXISTS round_xp_cap_99,
    DROP COLUMN IF EXISTS round_xp_cap_199,
    DROP COLUMN IF EXISTS round_xp_cap_299,
    DROP COLUMN IF EXISTS round_xp_cap_349,
    DROP COLUMN IF EXISTS round_xp_cap_398,
    DROP COLUMN IF EXISTS round_xp_cap_double_99,
    DROP COLUMN IF EXISTS round_xp_cap_double_199,
    DROP COLUMN IF EXISTS round_xp_cap_double_299,
    DROP COLUMN IF EXISTS round_xp_cap_double_349,
    DROP COLUMN IF EXISTS round_xp_cap_double_398;
