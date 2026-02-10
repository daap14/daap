-- databases.status must be a known value
ALTER TABLE databases ADD CONSTRAINT chk_databases_status
  CHECK (status IN ('provisioning', 'ready', 'error', 'deleting', 'deleted'));

-- Non-superusers must have a team
ALTER TABLE users ADD CONSTRAINT chk_users_superuser_team
  CHECK (is_superuser = TRUE OR team_id IS NOT NULL);

-- Revocation must be after creation
ALTER TABLE users ADD CONSTRAINT chk_users_revoked_after_created
  CHECK (revoked_at IS NULL OR revoked_at >= created_at);
