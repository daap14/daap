CREATE TABLE tiers (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(63) NOT NULL UNIQUE,
    description TEXT NOT NULL DEFAULT '',
    instances INT NOT NULL DEFAULT 1
        CHECK (instances >= 1 AND instances <= 10),
    cpu VARCHAR(20) NOT NULL DEFAULT '500m',
    memory VARCHAR(20) NOT NULL DEFAULT '512Mi',
    storage_size VARCHAR(20) NOT NULL DEFAULT '1Gi',
    storage_class VARCHAR(255) NOT NULL DEFAULT '',
    pg_version VARCHAR(10) NOT NULL DEFAULT '16',
    pool_mode VARCHAR(20) NOT NULL DEFAULT 'transaction'
        CHECK (pool_mode IN ('transaction', 'session', 'statement')),
    max_connections INT NOT NULL DEFAULT 100
        CHECK (max_connections >= 10 AND max_connections <= 10000),
    destruction_strategy VARCHAR(20) NOT NULL DEFAULT 'hard_delete'
        CHECK (destruction_strategy IN ('freeze', 'archive', 'hard_delete')),
    backup_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_tiers_name ON tiers (name);
