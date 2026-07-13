# Swarmery

Local control plane for Claude Code agent systems: Go daemon + embedded React SPA.
Parses session transcripts from `~/.claude/projects/` into SQLite and serves a
dashboard at `http://localhost:7777`.

- Design doc: [swarmery-design.md](swarmery-design.md)
- Implementation plan: [docs/plan/00-plan.md](docs/plan/00-plan.md)
- UI reference: [docs/design/swarmery-ui-mockup.html](docs/design/swarmery-ui-mockup.html)

## Status
Pre-MVP. Phase 1 (observation) in progress — see plan.

## Excluding throwaway projects

Spike/e2e runs under `/tmp` would otherwise pollute the dashboards. The
`--exclude-projects` flag (env `SWARMERY_EXCLUDE`, default
`/tmp/*,/private/tmp/*`) takes comma-separated path globs; a cwd is excluded
when a glob matches it or any ancestor directory. Both tracking channels
honor it:

- the **JSONL scanner** skips matching project dirs on backfill, rescan, and
  fsnotify tail — deleted data cannot rescan itself back in;
- the **hooks channel** still serves permission requests from excluded cwds
  (the fail-open decision flow is untouched: the daemon answers 204 and the
  shim falls back to the native dialog), but persists no session/project rows.

Exclusion gates row *creation* only — rows that already exist are never
deleted by code; remove them with a one-off SQL cleanup. Set
`SWARMERY_EXCLUDE=''` to disable.
