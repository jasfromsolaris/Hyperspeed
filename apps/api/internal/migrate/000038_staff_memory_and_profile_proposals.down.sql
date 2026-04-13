DROP INDEX IF EXISTS idx_sa_profile_proposals_org_sa_status;
DROP TABLE IF EXISTS service_account_profile_proposals;

DROP INDEX IF EXISTS idx_staff_memory_procedures_org_sa_updated;
DROP TABLE IF EXISTS staff_memory_procedures;

DROP INDEX IF EXISTS idx_staff_memory_facts_search;
DROP INDEX IF EXISTS idx_staff_memory_facts_active;
DROP INDEX IF EXISTS idx_staff_memory_facts_org_sa_created;
DROP TABLE IF EXISTS staff_memory_facts;

DROP INDEX IF EXISTS idx_staff_memory_episodes_search;
DROP INDEX IF EXISTS idx_staff_memory_episodes_org_sa_created;
DROP TABLE IF EXISTS staff_memory_episodes;

DROP INDEX IF EXISTS idx_staff_memory_runs_org_sa_created;
DROP TABLE IF EXISTS staff_memory_runs;

DROP TYPE IF EXISTS service_account_profile_proposal_status;
