-- QuestInfo.Arch.Cristal: how far the four-stage Arch crystal quest has gone
-- (0..4). Beyond blocking a repeat, this counter is what rebuilds the AC those
-- quests grant on every login — BaseScore.Ac is derived, not stored, so without
-- it the +30 and +20 would silently vanish on relog.
ALTER TABLE character
    ADD COLUMN arch_cristal SMALLINT NOT NULL DEFAULT 0;
