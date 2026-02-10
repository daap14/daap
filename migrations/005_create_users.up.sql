CREATE TABLE users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(255) NOT NULL,
    team_id UUID REFERENCES teams(id) ON DELETE RESTRICT,
    is_superuser BOOLEAN NOT NULL DEFAULT FALSE,
    api_key_prefix VARCHAR(8) NOT NULL,
    api_key_hash VARCHAR(255) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    revoked_at TIMESTAMPTZ
);
CREATE UNIQUE INDEX idx_users_superuser ON users (is_superuser) WHERE is_superuser = TRUE;
CREATE INDEX idx_users_api_key_prefix ON users (api_key_prefix) WHERE revoked_at IS NULL;
CREATE INDEX idx_users_team_id ON users (team_id);
