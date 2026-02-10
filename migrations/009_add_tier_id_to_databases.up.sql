ALTER TABLE databases
    ADD COLUMN tier_id UUID REFERENCES tiers(id) ON DELETE RESTRICT;

CREATE INDEX idx_databases_tier_id ON databases (tier_id);
