-- Step 1: Add back the VARCHAR column
ALTER TABLE databases ADD COLUMN owner_team VARCHAR(255);

-- Step 2: Populate from teams
UPDATE databases d
SET owner_team = t.name
FROM teams t
WHERE d.owner_team_id = t.id;

-- Step 3: Set NOT NULL
ALTER TABLE databases ALTER COLUMN owner_team SET NOT NULL;

-- Step 4: Recreate original index
CREATE INDEX idx_databases_owner_team ON databases (owner_team);

-- Step 5: Drop new index and FK
DROP INDEX IF EXISTS idx_databases_owner_team_id;
ALTER TABLE databases DROP CONSTRAINT IF EXISTS fk_databases_owner_team;

-- Step 6: Drop UUID column
ALTER TABLE databases DROP COLUMN owner_team_id;
