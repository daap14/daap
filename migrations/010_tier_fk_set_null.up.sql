ALTER TABLE databases DROP CONSTRAINT databases_tier_id_fkey;
ALTER TABLE databases ADD CONSTRAINT databases_tier_id_fkey
    FOREIGN KEY (tier_id) REFERENCES tiers(id) ON DELETE SET NULL;
