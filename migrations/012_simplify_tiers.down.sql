DROP INDEX IF EXISTS idx_tiers_blueprint_id;

ALTER TABLE tiers
    ADD COLUMN instances INT NOT NULL DEFAULT 1 CHECK (instances >= 1 AND instances <= 10),
    ADD COLUMN cpu VARCHAR(20) NOT NULL DEFAULT '500m',
    ADD COLUMN memory VARCHAR(20) NOT NULL DEFAULT '512Mi',
    ADD COLUMN storage_size VARCHAR(20) NOT NULL DEFAULT '1Gi',
    ADD COLUMN storage_class VARCHAR(255) NOT NULL DEFAULT '',
    ADD COLUMN pg_version VARCHAR(10) NOT NULL DEFAULT '16',
    ADD COLUMN pool_mode VARCHAR(20) NOT NULL DEFAULT 'transaction' CHECK (pool_mode IN ('transaction', 'session', 'statement')),
    ADD COLUMN max_connections INT NOT NULL DEFAULT 100 CHECK (max_connections >= 10 AND max_connections <= 10000);

ALTER TABLE tiers DROP COLUMN IF EXISTS blueprint_id;
