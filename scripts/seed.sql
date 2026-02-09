-- Seed data for DAAP databases table
-- Idempotent: uses ON CONFLICT to skip existing rows

INSERT INTO databases (id, name, owner_team, purpose, namespace, cluster_name, pooler_name, status, host, port, secret_name, created_at, updated_at)
VALUES
  (gen_random_uuid(), 'user-service-db', 'platform', 'User service PostgreSQL', 'default', 'daap-user-service-db', 'daap-user-service-db-pooler', 'ready', 'daap-user-service-db-pooler.default.svc.cluster.local', 5432, 'daap-user-service-db-app', NOW(), NOW()),
  (gen_random_uuid(), 'order-service-db', 'commerce', 'Order processing database', 'default', 'daap-order-service-db', 'daap-order-service-db-pooler', 'ready', 'daap-order-service-db-pooler.default.svc.cluster.local', 5432, 'daap-order-service-db-app', NOW(), NOW()),
  (gen_random_uuid(), 'analytics-db', 'data-team', 'Analytics and reporting', 'default', 'daap-analytics-db', 'daap-analytics-db-pooler', 'provisioning', NULL, NULL, NULL, NOW(), NOW()),
  (gen_random_uuid(), 'staging-db', 'platform', 'Staging environment', 'staging', 'daap-staging-db', 'daap-staging-db-pooler', 'error', NULL, NULL, NULL, NOW(), NOW())
ON CONFLICT (name) WHERE deleted_at IS NULL DO NOTHING;
