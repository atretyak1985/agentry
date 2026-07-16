-- 0014: operational-hygiene columns on sessions (one migration, two features).
--
-- pruned: `swarmery prune` rolled this session's raw rows (turns/events/
--   file_changes) into daily_rollups and deleted them; the header row stays
--   browsable in the list and detail. 0 = raw rows intact.
--
-- outcome: manual verdict set from the dashboard (PATCH /api/sessions/{id});
--   NULL = not judged. Feeds the analytics success-rate column. The CHECK
--   applies to new writes only — existing rows are NULL, which passes.
ALTER TABLE sessions ADD COLUMN pruned INTEGER NOT NULL DEFAULT 0;
ALTER TABLE sessions ADD COLUMN outcome TEXT CHECK(outcome IN ('success','fail','abandoned'));
