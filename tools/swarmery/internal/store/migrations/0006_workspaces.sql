-- 0006: phase 3.5 Workspaces (E-lite subset of swarmery-design.md §7.2) —
-- the intent↔telemetry bridge. Additive only: a workspaces registry, extra
-- columns on tasks for disk-ingested cards, and the task_sessions link table.
-- Deliberately OMITS task_phases and task_artifacts (full Agent E scope).

-- ============ Workspaces (phase 3.5): intent ↔ telemetry bridge ============

CREATE TABLE workspaces (
    id           INTEGER PRIMARY KEY,
    slug         TEXT NOT NULL UNIQUE,       -- project name in the workspace repo
    root_path    TEXT NOT NULL,              -- $AGENT_WORKSPACE_ROOT/<slug>
    code_path    TEXT,                       -- overlay/project.json → codePath
    project_id   INTEGER REFERENCES projects(id),  -- join to the project registry
                 -- (codePath ↔ projects.path, symlink/trailing-slash normalized)
    display_name TEXT,
    last_scanned TEXT
);

-- workspace tasks live in the same tasks table (additive columns):
ALTER TABLE tasks ADD COLUMN source TEXT NOT NULL DEFAULT 'queue';
                 -- 'queue' (created from the dashboard) | 'workspace' (disk ingest)
ALTER TABLE tasks ADD COLUMN external_id TEXT;   -- yyyy-mm-dd-slug (card task id)
ALTER TABLE tasks ADD COLUMN workspace_id INTEGER REFERENCES workspaces(id);
ALTER TABLE tasks ADD COLUMN archived_at TEXT;   -- card moved into archive/
CREATE UNIQUE INDEX idx_tasks_workspace_external
    ON tasks(workspace_id, external_id)
    WHERE workspace_id IS NOT NULL;              -- UNIQUE(workspace_id, external_id)

CREATE TABLE task_sessions (
    task_id     INTEGER NOT NULL REFERENCES tasks(id),
    session_id  INTEGER NOT NULL REFERENCES sessions(id),
    link_source TEXT NOT NULL,                -- explicit | heuristic
    confidence  REAL,                         -- 0..1 for heuristic links
    PRIMARY KEY (task_id, session_id)
);
