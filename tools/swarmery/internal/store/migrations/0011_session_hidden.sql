-- 0011: soft-hide sessions. A session flagged hidden=1 is filtered out of the
-- sessions list (via DELETE /api/sessions/{id}); its row and .jsonl transcript
-- are kept. Because a session row is INSERTed once and never rewritten by
-- re-ingest, the flag survives rescans, and it stays reversible (hidden=0).
-- Additive with a default; existing rows are unaffected.
ALTER TABLE sessions ADD COLUMN hidden INTEGER NOT NULL DEFAULT 0;
