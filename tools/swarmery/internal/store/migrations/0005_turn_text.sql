-- 0005: turns.text — the turn's human-readable prose for the Chat tab.
-- User turns store the prompt text; assistant turns store the concatenation
-- of all `text` content blocks across the turn's split API-message lines
-- (separated by a blank line), excluding thinking and tool_use blocks.
-- Never truncated. Additive only — NULL for rows ingested before this
-- migration (backfill with `swarmery backfill --rebuild-text`).
ALTER TABLE turns ADD COLUMN text TEXT;
