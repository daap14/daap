-- Step 1: Add nullable UUID column
ALTER TABLE databases ADD COLUMN owner_team_id UUID;

-- Step 2: Populate from teams by matching name
UPDATE databases d
SET owner_team_id = t.id
FROM teams t
WHERE d.owner_team = t.name;

-- Step 3: Auto-create placeholder teams for orphaned names
INSERT INTO teams (name, role)
SELECT DISTINCT d.owner_team, 'platform'
FROM databases d
WHERE d.owner_team_id IS NULL
  AND d.owner_team IS NOT NULL
  AND d.owner_team != ''
ON CONFLICT (name) DO NOTHING;

-- Step 4: Re-populate after placeholder creation
UPDATE databases d
SET owner_team_id = t.id
FROM teams t
WHERE d.owner_team = t.name
  AND d.owner_team_id IS NULL;

-- Step 5: Set NOT NULL
ALTER TABLE databases ALTER COLUMN owner_team_id SET NOT NULL;

-- Step 6: Add FK constraint
ALTER TABLE databases ADD CONSTRAINT fk_databases_owner_team
  FOREIGN KEY (owner_team_id) REFERENCES teams(id) ON DELETE RESTRICT;

-- Step 7: Drop old column and index
DROP INDEX IF EXISTS idx_databases_owner_team;
ALTER TABLE databases DROP COLUMN owner_team;

-- Step 8: Create filtered index on new column
CREATE INDEX idx_databases_owner_team_id ON databases (owner_team_id) WHERE deleted_at IS NULL;
