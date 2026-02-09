CREATE TABLE databases (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(63) NOT NULL,
    owner_team VARCHAR(255) NOT NULL,
    purpose TEXT NOT NULL DEFAULT '',
    namespace VARCHAR(255) NOT NULL DEFAULT 'default',
    cluster_name VARCHAR(255) NOT NULL,
    pooler_name VARCHAR(255) NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'provisioning',
    host VARCHAR(255),
    port INTEGER,
    dbname VARCHAR(255),
    username VARCHAR(255),
    password VARCHAR(255),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);

CREATE UNIQUE INDEX idx_databases_name_active ON databases (name) WHERE deleted_at IS NULL;
CREATE INDEX idx_databases_owner_team ON databases (owner_team);
CREATE INDEX idx_databases_status ON databases (status);
