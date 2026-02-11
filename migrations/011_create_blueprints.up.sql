CREATE TABLE blueprints (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(63) NOT NULL UNIQUE,
    provider VARCHAR(63) NOT NULL,
    manifests TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_blueprints_name ON blueprints (name);
CREATE INDEX idx_blueprints_provider ON blueprints (provider);
