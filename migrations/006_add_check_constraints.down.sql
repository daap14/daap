ALTER TABLE users DROP CONSTRAINT IF EXISTS chk_users_revoked_after_created;
ALTER TABLE users DROP CONSTRAINT IF EXISTS chk_users_superuser_team;
ALTER TABLE databases DROP CONSTRAINT IF EXISTS chk_databases_status;
