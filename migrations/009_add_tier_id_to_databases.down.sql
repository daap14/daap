DROP INDEX IF EXISTS idx_databases_tier_id;
ALTER TABLE databases DROP COLUMN IF EXISTS tier_id;
