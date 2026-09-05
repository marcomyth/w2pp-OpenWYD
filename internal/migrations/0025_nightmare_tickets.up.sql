-- MobExtra.NT: the Pesadelo Arcano entries a character holds (pesadelo-plan.md).
--
-- Escritura do Pesadelo grants 13; every admission to the A tier spends one, for
-- the party leader and each Celestial member. Kept on the character rather than
-- in the raw 552-byte MobExtra blob, like the rest of the progression this port
-- has moved into Postgres (0020_arch_celestial_progression).
--
-- No backfill: nobody could enter the Arcano tier before this shipped, so every
-- existing character legitimately starts at zero and buys the Escritura.
ALTER TABLE character
    ADD COLUMN nightmare_tickets INTEGER NOT NULL DEFAULT 0;
