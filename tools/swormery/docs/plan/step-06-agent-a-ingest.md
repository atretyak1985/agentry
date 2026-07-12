# Step 06 — Agent A: ingest hardening (live pipeline)

## Header

| Field | Value |
|---|---|
| Phase | 3 — Parallel wave |
| Duration | 1 agent session, ~3–5 h (LOW-CONFIDENCE — watcher edge cases vary) |
| Type | Agent session (code, runs in parallel with steps 07, 08) |
| Risk | Medium — filesystem watching + dedup correctness |
| Dependencies | Step 05 gate PASS; worktree `/Volumes/Work/swarmery-wt-ingest` (work in `tools/swormery`) |

## Goal

Turn one-shot ingest into a resilient live pipeline: full backfill of
`~/.claude/projects/**`, fsnotify tail with persisted offsets, dedup, session-status
ticker, and a WS endpoint broadcasting ingest events per a documented protocol.

## Automation

Fresh Claude Code session in `/Volumes/Work/swarmery-wt-ingest/tools/swormery` (worktree, branch
`feat/swormery-ingest`). Runs concurrently with Agents B and C.

## Agent Prompt

```
Reference: docs/plan/step-06-agent-a-ingest.md

Context:
Репозиторій Swormery після T1 (скелет працює). Прочитай swormery-design.md,
docs/jsonl-format.md, internal/ingest і internal/store. Працюєш у гілці
feat/swormery-ingest (worktree). Паралельно інші агенти роблять фронтенд і метрики —
НЕ чіпай web/ та internal/api, окрім додавання /api/ws у відведений блок
"// wave A: WS" в internal/api/routes.go.

Tasks (живий pipeline):
1. Сканер: обхід ~/.claude/projects/**, реєстрація проєктів (slug → path),
   повний backfill усіх .jsonl.
2. Watcher: fsnotify на директорії + fallback-rescan кожні 2с (конфіг).
   Інкрементальний tail: читати з byte_offset із file_offsets, оновлювати
   після кожного батча; враховуй inode (файл міг бути пересозданий).
3. Дедуплікація: dedup_key = session_uuid + ":" + line_number (або hash
   рядка, якщо номер ненадійний). Повторний повний backfill НЕ створює
   жодного дубліката.
4. Статуси сесій: active (<2 хв від останнього запису), idle (<30 хв),
   completed; пороги в конфіг. Фоновий тікер перераховує статуси.
   ТІЛЬКИ ці три статуси (waiting_approval/killed — Фаза 2).
5. Event bus: внутрішній канал подій ingest → підписники. WS-ендпоінт
   /api/ws (github.com/coder/websocket) транслює session_started |
   session_updated | event_appended — імена і payload СТРОГО за
   WSMessage у web/src/api/types.ts (заморожено). Формат повідомлень
   задокументуй у docs/ws-protocol.md — фронтенд-агент працює по ньому.
6. Стійкість: жодна помилка одного файлу не зупиняє pipeline; метрики
   інджесту (файлів, рядків, помилок) у лог.

Boundaries:
- НЕ змінюй наявні таблиці зі swormery-design.md; нові — тільки службові
  (адитивна міграція).
- НЕ чіпай web/ і types.ts (потреби → web/CONTRACT-REQUESTS.md).
- Нові залежності: тільки fsnotify і coder/websocket (бюджет 3 не порушено).
- Тести обовʼязкові: tail з offset після "рестарту", повторний backfill
  без дублікатів, битий файл не ронить сканер.

Output / Validation:
go vet + go test зелені. Живий тест: запусти serve, у ДРУГОМУ терміналі
запусти коротку сесію claude в будь-якому проєкті — покажи в лозі, що події
підхопились у реальному часі (<3с лаг). Conventional commits у feat/swormery-ingest.
Заповни Completion Report у docs/plan/step-06-agent-a-ingest.md (у worktree).
```

## Detailed Instructions

- Offset semantics: only advance `byte_offset` after the batch transaction commits —
  crash between read and commit must re-read, and dedup absorbs the replay.
- inode check: if `stat` inode differs from stored → reset offset to 0 (file
  recreated); dedup prevents duplicates on the re-read.
- Status ticker also emits `session_updated` on WS so the frontend list stays live.
- macOS gotcha: fsnotify on `~/.claude/projects` needs per-directory watches for new
  project dirs — watch the root and add watches on create; the 2 s rescan is the net.

## Success Criteria

- [ ] `go test ./...` green incl. new tests: offset-resume, dedup-on-rebackfill (0 dupes), corrupt-file-survival
- [ ] Full backfill of this machine completes; log reports files/lines/errors counts
- [ ] Live `claude` session events visible in log < 3 s after they happen
- [ ] `docs/ws-protocol.md` exists and matches frozen `WSMessage` type exactly
- [ ] Diff touches only `internal/ingest`, `internal/store` (additive), `routes.go` wave-A block, `docs/`, `go.mod`

## Navigation

Previous: [step-05-quality-gate-contract-freeze.md](step-05-quality-gate-contract-freeze.md) · Next: [step-07-agent-b-frontend.md](step-07-agent-b-frontend.md) (parallel) · Index: [00-plan.md](00-plan.md)

### Completion Report

```
Date/agent: · Branch head SHA: · Backfill stats: · Live-tail lag observed: · CONTRACT-REQUESTS entries:
```
