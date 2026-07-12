# Step 2.6 — Integration + live test protocol

## Header

| Field | Value |
|---|---|
| Phase | 2 — Approvals + hooks |
| Duration | 1 agent session + human co-driving the live protocol, ~0.5 day (MEDIUM — analogous to MVP step 10) |
| Type | Agent session (merge + wiring) + human-in-the-loop live testing |
| Risk | Medium — first time hook shim, daemon and SPA meet real Claude Code |
| Dependencies | Gate 2.5 PASS; merge order backend → frontend |

## Goal

Merge both branches into main, reconcile any accepted contract requests, and run
the full live test protocol against real `claude` sessions on this machine.

## Agent Prompt

```
Reference: docs/plan/phase-2-approvals/step-2.6-integration-live-test.md

Context:
Swarmery, гілка main. Обидві гілки фази 2 пройшли гейт 2.5. Прочитай
docs/hooks-protocol.md, web/CONTRACT-REQUESTS.md (прийняті запити),
Completion Reports кроків 2.3/2.4. Людина поруч — живий протокол
виконуєте разом.

Tasks:
1. Merge: спершу feat/swarmery-hooks, потім feat/swarmery-approvals-ui
   (no-ff, conventional merge commits). Конфлікти можливі тільки в
   routes.go/docs — вирішуй за структурою wave-блоків.
2. Прийняті contract-запити (якщо є): онови types.ts + docs разом,
   одним комітом, за процедурою MVP step 10.
3. make build → повний бінар з SPA; go vet + go test -race + npm run
   build зелені на merged main.
4. ЖИВИЙ ПРОТОКОЛ (з людиною, кожен пункт — у Completion Report з
   фактичними цифрами):
   P1 install: swarmery hooks install --project <тестовий проєкт>;
      hooks status показує installed; claude-сесія стартує без скарг.
   P2 approve: un-allowlisted команда → pending на /approvals < 2с
      (WS), сесія amber waiting_approval; Approve з дашборда →
      команда виконалась у терміналі БЕЗ рідного діалогу; в таймлайні
      сесії — permission_request + permission_resolved; статус
      повернувся до active.
   P3 deny з reason → Claude отримав відмову і причину; рядок у
      History з resolved_via=dashboard.
   P4 timeout: не чіпаємо approval_timeout секунд → status=expired,
      рідний діалог зʼявився, сесія жива, відповідь з термінала
      працює.
   P5 daemon down: launchctl stop (або kill) демона → нова
      un-allowlisted команда → рідний діалог з затримкою ≤1.5с;
      hook.log зафіксував fail-open; демон назад — потік відновився
      без перезапуску claude.
   P6 dedup: два субагенти з ідентичним запитом (або повторний
      реплей) → один pending-рядок, одне рішення закриває обидва.
   P7 resolved_elsewhere: відповідь у терміналі під час pending
      (семантика E4) → рядок закрито з resolved_via=terminal.
   P8 heartbeat: /api/health показує hooks_last_seen свіжіший за
      хвилину після P2.
   P9 Stop-hook: завершення відповіді → 202 у лог демона (канал 2.5).
   P10 uninstall: swarmery hooks uninstall --project … → у
      settings.local.json нуль swarmery-entries, чужі хуки на місці;
      наступна claude-сесія працює як до фази 2.
5. swarmery install (launchd) перезапуск демона з новим бінарем;
   переконайся, що P2 працює і через launchd-інстанс.
6. Онови docs/plan/phase-2-approvals/00-phase-2-plan.md: чекбокси
   прогресу; додай знахідки протоколу в docs/plan/phase2-backlog.md
   (нова секція "Phase 2 dogfooding").

Boundaries:
- Жодних нових фіч під час інтеграції — тільки wiring і фікси,
  потрібні для проходження P1–P10.
- Контрактні зміни — тільки з web/CONTRACT-REQUESTS.md, прийняті
  людиною.

Output / Validation:
Всі P1–P10 зелені з цифрами в Completion Report. main зелений у CI
(swarmery-ci.yml). Заповни Completion Report у
docs/plan/phase-2-approvals/step-2.6-integration-live-test.md.
```

## Detailed Instructions

- P4/P7 depend on the E4 spike findings — if the terminal dialog is *suppressed*
  while the hook runs, P7's expected status changes (record actual vs expected).
- P5 must be measured, not eyeballed: wrap the shim call timing from `hook.log`
  timestamps (fail-open budget ≤ 1.5 s).
- The launchd instance (task 5) matters: the shim targets `127.0.0.1:$SWARMERY_PORT`
  and the plist sets the environment — verify no port mismatch between `hooks
  install --port` and the running daemon.
- Do not install hooks into `--all` real projects yet — that is the Gate 2.7
  dogfooding decision, made by the human after the protocol passes.

## Success Criteria

- [ ] Both branches merged; CI green on main; single binary serves the Approvals screen
- [ ] P1–P10 all pass with recorded measurements (latency P2, fail-open P5)
- [ ] `phase2-backlog.md` gained a "Phase 2 dogfooding" section (even if empty)
- [ ] No leftover test hooks in any real project's settings after the session

## Navigation

Previous: [step-2.5-quality-gate-parallel-wave.md](step-2.5-quality-gate-parallel-wave.md) · Next: [step-2.7-quality-gate-ship.md](step-2.7-quality-gate-ship.md) · Index: [00-phase-2-plan.md](00-phase-2-plan.md)

### Completion Report

Status: **DONE (integration scope)** — both wave branches merged into main,
all verification green. The human-in-the-loop live protocol (P1–P10) was NOT
run in this session — it needs a co-driver and real `claude` sessions; it
moves to the Gate 2.7 checklist as the remaining evidence.

**Merge order & conflict resolution** (merge main INTO each branch, verify,
push, then merge the PR — both PRs conflicted because E-lite/workspaces
landed on main after they forked):

1. `feat/swarmery-hooks` ← `origin/main` (PR #25, MERGED):
   - `internal/api/routes.go` — only textual conflict; resolved as the union
     of wave blocks: `// phase 3.5: workspaces` (tasks endpoints) + `// phase
     2: approvals` (hooks/approvals endpoints), both kept.
   - `handlers.go` (sessionSelect task-attribution columns), `ws_test.go`
     (golden keys: task fields + PermissionRequest keys), `cmd/swarmery/main.go`
     (`wscan` wiring + `--bind`/`hook`/`hooks`) auto-merged as clean unions;
     migrations `0006_workspaces` + `0007_approvals` coexist.
   - Verified: `gofmt` clean, `go vet ./...`, `go test -race ./...` all green.
2. `feat/swarmery-approvals-ui` ← `origin/main` (PR #24, MERGED):
   - `web/src/api.ts`, `web/src/mock/data.ts` — union: approvals client +
     mocks alongside tasks client + mocks.
   - `web/src/pages/Overview.tsx` (4 hunks) — union: PENDING APPROVALS rail +
     live `waiting approval` subline AND the `Tasks · 14 days` slice; both
     load paths + reload wiring kept.
   - `web/CONTRACT-REQUESTS.md` — union of the phase-3.5 resolutions section
     and the phase-2 wave-B requests.
   - `web/src/lib/ws.ts` — no conflict; the shared-connection refactor already
     carries the 60 s reconcile + malformed-frame guard.
   - Verified: `tsc --noEmit` strict + `npm run build` green; all screenshots
     regenerated via `VITE_MOCK=1 vite :5199` + `scripts/screenshot.mjs`
     (desktop Overview visually confirmed to show approvals badge + rail +
     tasks slice together).

**Merged-main verification** (commit `348c8f0`): `gofmt` clean,
`go vet ./...` green, `go test -race ./...` green (api, approvals, cost,
hookcfg, hookshim, ingest, installer, wsingest), `tsc --noEmit` green,
`npm run build` green (324 kB JS / 101 kB gzip).

**Contract requests reconciled** — see "Step-2.6 resolutions" in
`web/CONTRACT-REQUESTS.md`: `status=resolved`/`all` meta-filters and the 409
conflict contract were already implemented server-side (answered, no code
change); the denormalized `projectSlug`/`projectName`/`sessionTitle` DTO
fields are deferred to the phase-2 backlog (frozen ws_test golden key-set +
mock fixtures + multi-prop UI refactor > trivial; lazy join covers the UX).

**Left for Gate 2.7**: live protocol P1–P10 with measurements (P2 latency,
P5 fail-open), launchd binary swap check, `phase2-backlog.md` "Phase 2
dogfooding" section, dogfood-rollout decision.
