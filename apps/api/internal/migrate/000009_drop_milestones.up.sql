-- Remove milestones feature.

-- Drop FK from tasks -> milestones then drop the column.
ALTER TABLE tasks
    DROP CONSTRAINT IF EXISTS tasks_milestone_id_fkey;

ALTER TABLE tasks
    DROP COLUMN IF EXISTS milestone_id;

-- Drop milestones table + index.
DROP TABLE IF EXISTS milestones;

