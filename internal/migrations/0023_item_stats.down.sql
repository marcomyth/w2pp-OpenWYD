-- Undo the item base stat overrides.
--
-- Dropping the table restores ItemList.csv as the only source of an item's
-- effects, which is what an un-overridden item already uses — so the game needs
-- nothing beyond its next boot to be consistent again.
DROP INDEX IF EXISTS item_stat_updated_at_idx;
DROP TABLE IF EXISTS item_stat;
