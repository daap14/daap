ALTER TABLE tiers
    ADD COLUMN blueprint_id UUID REFERENCES blueprints(id) ON DELETE RESTRICT;

ALTER TABLE tiers
    DROP COLUMN IF EXISTS instances,
    DROP COLUMN IF EXISTS cpu,
    DROP COLUMN IF EXISTS memory,
    DROP COLUMN IF EXISTS storage_size,
    DROP COLUMN IF EXISTS storage_class,
    DROP COLUMN IF EXISTS pg_version,
    DROP COLUMN IF EXISTS pool_mode,
    DROP COLUMN IF EXISTS max_connections;

CREATE INDEX idx_tiers_blueprint_id ON tiers (blueprint_id);
