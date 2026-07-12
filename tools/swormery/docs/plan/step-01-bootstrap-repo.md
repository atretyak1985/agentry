# Step 01 — Bootstrap the swormery repository

## Header

| Field | Value |
|---|---|
| Phase | 1 — Bootstrap & JSONL spike |
| Duration | ~15–30 min (HIGH confidence — plain shell commands) |
| Type | Human (or one short agent session) |
| Risk | Low |
| Dependencies | None |

## Goal

Create the standalone git repository `/Volumes/Work/swarmery/tools/swormery` with the design docs,
UI mockup, and this plan copied in, so every subsequent agent session has its
sources of truth inside the repo it works in.

## Automation

Human at a terminal, or a single Claude Code session in `/Volumes/Work` with default
permissions. No subagents needed.

## Agent Prompt

```
Reference: docs/plan/step-01-bootstrap-repo.md (this file, once copied)

Context:
Я створюю новий репозиторій /Volumes/Work/swarmery/tools/swormery для Go+React дашборда.
Вихідні доки лежать у /Volumes/Work/swarmery-workspace/swarmery/workspace/working/2026/07/12/swormery-control-plane/docs/ (план — у ../plan/).
УВАГА: це НЕ репозиторій swarmery (plugin marketplace) — нічого там не змінюй.

Tasks:
1. mkdir -p /Volumes/Work/swarmery/tools/swormery/docs/design /Volumes/Work/swarmery/tools/swormery/docs/plan
   && cd /Volumes/Work/swarmery/tools/swormery && git init -b main
2. Скопіюй з /Volumes/Work/swarmery-workspace/swarmery/workspace/working/2026/07/12/swormery-control-plane/:
   - docs/swormery-design.md → ./swormery-design.md
   - docs/swormery-ui-mockup.html → docs/design/
   - plan/*.md → docs/plan/   (00-plan.md і всі step-файли)
3. Створи .gitignore і README.md за зразками з Detailed Instructions нижче.
4. git add -A && git commit -m "chore: bootstrap repo with design docs and implementation plan"

Output: шлях до репо, git log --oneline, ls docs/plan.
Наприкінці заповни Completion Report у docs/plan/step-01-bootstrap-repo.md.
```

## Detailed Instructions

`.gitignore`:

```gitignore
swormery
*.db
*.db-wal
*.db-shm
node_modules/
web/dist/
.DS_Store
```

`README.md`:

```markdown
# Swormery

Local control plane for Claude Code agent systems: Go daemon + embedded React SPA.
Parses session transcripts from `~/.claude/projects/` into SQLite and serves a
dashboard at `http://localhost:7777`.

- Design doc: [swormery-design.md](swormery-design.md)
- Implementation plan: [docs/plan/00-plan.md](docs/plan/00-plan.md)
- UI reference: [docs/design/swormery-ui-mockup.html](docs/design/swormery-ui-mockup.html)

## Status
Pre-MVP. Phase 1 (observation) in progress — see plan.
```

Gotchas:
- Source path is `/Volumes/Work/swarmery-workspace/swarmery/workspace/working/2026/07/12/swormery-control-plane/`
  (source docs in `docs/`, plan in `plan/`) — the superseded draft referenced a
  non-existent `temp_files/` path (fix F1).
- Do NOT copy the old `swormery-implementation-plan.md` — this plan supersedes it.
- Do NOT copy `swormery-mvp-prompt.md` / `swormery-agent-tasks.md` — their content is
  already folded into the step files.

## Success Criteria

- [ ] `git -C /Volumes/Work/swarmery/tools/swormery log --oneline` shows exactly 1 commit on `main`
- [ ] `swormery-design.md` in repo root; mockup in `docs/design/`; ≥13 files in `docs/plan/`
- [ ] `.gitignore` and `README.md` match templates above
- [ ] Nothing under `/Volumes/Work/swarmery` was modified (`git -C /Volumes/Work/swarmery status` clean)

## Navigation

Previous: — · Next: [step-02-jsonl-spike.md](step-02-jsonl-spike.md) · Index: [00-plan.md](00-plan.md)

### Completion Report

```
Date/agent: 2026-07-12 / main-session controller · Commit SHA: 84e0126 (standalone repo, now defunct) · Deviations: none (13 plan files copied, source docs/plan paths already relocated to workspace/working) · Notes for next step: repo clean, spike candidates verified present in ~/.claude/projects
RELOCATED 2026-07-12: owner decision — the standalone /Volumes/Work/swormery repo was folded into swarmery as tools/swormery (branch feat/swormery-control-plane); module renamed to github.com/atretyak1985/swarmery/tools/swormery; standalone repo deleted.
```
