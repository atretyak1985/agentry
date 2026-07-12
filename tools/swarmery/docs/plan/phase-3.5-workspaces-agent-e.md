# Phase 3.5 — Agent E: Workspaces (intent ↔ telemetry bridge)

## Header

| Field | Value |
|---|---|
| Phase | 3.5 — Workspaces (design doc §7, §4 п.3.5) |
| Duration | 1–2 agent sessions (MEDIUM — ingester і linking незалежні від UI) |
| Type | Agent session (code) — **два репозиторії**: основна робота в `tools/swarmery` + ОКРЕМИЙ малий PR у `plugins/core` (SessionStart hook + semver bump + CHANGELOG) |
| Risk | Medium — read-only інжест чужого репо, path-traversal на artifact API, толерантність до битих workspace-ів |
| Dependencies | Тільки MVP (фаза 1). **НЕ залежить від Approvals** — може йти паралельно з фазою 2 |
| Branch | `feat/swarmery-workspaces` (tools/swarmery) + окрема гілка/PR для plugins/core |

## Goal

Зшити два світи: workspace-репо (`$AGENT_WORKSPACE_ROOT`, system of record
задач — ним керує `agent-work.sh`) і телеметрію Swarmery. Read-only ingester
індексує задачі/фази/артефакти, зшивка задача↔сесія (explicit через
SessionStart-hook + heuristic по cwd/часу), новий екран Workspaces з
per-task вартістю. Swarmery **ніколи не пише у workspace**.

## Prerequisites / ordering

- **Після MVP (фаза 1)**: потрібні лише `sessions`/`turns`/`events` і SPA.
  Approvals (фаза 2) НЕ потрібні — фази 2 і 3.5 можуть іти паралельно.
- **Живий workspace**: `/Volumes/Work/swarmery-workspace`
  (`AGENT_WORKSPACE_ROOT`) — для фінальної live-перевірки.
- **Синтетичний workspace**: фікстура в `testdata/` — для тестів
  (лічильники, heuristic link, path-traversal).

## Agent Prompt

```
Reference: docs/plan/phase-3.5-workspaces-agent-e.md

Context:
Репозиторій Swarmery (tools/swarmery): Go-демон + React SPA + SQLite, MVP
відвантажено. Прочитай swarmery-design.md §7 (Workspaces — принципи, DDL,
ingester, linking, UI) і §4 п.3.5, internal/ingest, internal/store,
internal/api, web/CONTRACT-REQUESTS.md. Працюєш у гілці
feat/swarmery-workspaces. Workspace-репо: $AGENT_WORKSPACE_ROOT
(/Volumes/Work/swarmery-workspace), структура
<slug>/workspace/{working,archive}/YYYY/MM/DD/<task-slug>/ з README.md
(картка `- **Field**: value`), plan/, logs/sessions.md,
overlay/project.json (поле codePath). Власник стану workspace —
agent-work.sh з plugins/core; Swarmery — тільки читач.

Objective:
1. Міграція (адитивна) — ДОСЛІВНО за swarmery-design.md §7.2:
   workspaces, ALTER tasks (source/external_id/workspace_id/archived_at
   + UNIQUE(workspace_id, external_id) через partial unique index),
   task_phases, task_artifacts, task_sessions. Наявні таблиці не чіпати.
2. internal/wsingest: сканер + fsnotify + періодичний rescan по
   $AGENT_WORKSPACE_ROOT/*/workspace/{working,archive}. Директорія задачі
   YYYY/MM/DD/<slug> → external_id=yyyy-mm-dd-slug; archive/ →
   archived_at. Толерантний парсинг README-картки (`- **Field**: value`;
   відсутнє поле → NULL, бита картка → задача все одно індексується,
   сканер не падає). Мапінг overlay codePath ↔ projects.path з
   нормалізацією symlink-ів і trailing-slash → workspaces.project_id.
   Спільний exclude-helper (project exclude-list) для ОБОХ колекторів —
   JSONL і workspace.
3. Зшивка задача↔сесія:
   a) explicit: читання таблиці logs/sessions.md картки → рядки
      task_sessions(link_source='explicit');
   b) heuristic: sessions.cwd ∈ workspaces.code_path ∧ перекриття
      часових вікон → link_source='heuristic' + confidence 0..1;
   c) ОКРЕМИЙ PR у plugins/core: SessionStart-hook дописує рядок
      `| <date> | <session_uuid> | | |` у logs/sessions.md активної
      картки, коли встановлено $AGENT_TASK_ID — через команду
      agent-work.sh, якщо така існує, інакше мінімальний append з flock.
      + semver bump плагіна + CHANGELOG. Це єдина зміна поза
      tools/swarmery.
4. API: GET /api/workspaces; GET /api/workspaces/{slug}/tasks?state=;
   GET /api/tasks/{id} (картка + phases + artifacts + sessions з Σ cost);
   GET /api/tasks/{id}/artifacts/{artifact_id} — markdown on-demand:
   шлях береться ТІЛЬКИ з task_artifacts (жодних шляхів із запиту),
   redaction секретів на виході (той самий фільтр, що на ingest).
5. UI: пункт Workspaces у sidebar; список workspace-ів (лічильники
   working/archive + 7-day $); задачі проєкту з phase stepper і
   вартістю = Σ сесій; картка задачі (README+SUMMARY render, фази,
   deep-links на сесії, артефакти, trace on-demand); heuristic-звʼязки
   пунктиром з confirm/reject (confirm → link_source='explicit' у БД,
   workspace НЕ чіпається); chip задачі в Session Detail.

Boundaries:
- Swarmery НІКОЛИ не пише у workspace-репо (єдиний виняток —
  SessionStart-hook у plugins/core, і той окремим PR).
- internal/ingest не чіпати, крім винесення спільного exclude-helper.
- types.ts — тільки адитивні зміни, через web/CONTRACT-REQUESTS.md.
- Толерантність до битих workspace-ів: жодна зламана картка/структура
  не валить сканер і не блокує решту проєктів.

Output / Validation:
go vet + go test зелені; npm run build зелений. Тести на фікстурному
workspace у testdata/: лічильники задач/фаз/артефактів, heuristic link
(cwd+вікно → confidence), path-traversal ../../ → 404. Live-тест:
інжест /Volumes/Work/swarmery-workspace — звірити лічильники задач по
проєктах з диском; скріншоти екрана Workspaces і картки задачі.
Conventional commits у feat/swarmery-workspaces. Заповни Completion
Report у docs/plan/phase-3.5-workspaces-agent-e.md.
```

## Success Criteria

- [ ] Міграція створює workspaces / task_phases / task_artifacts / task_sessions і ALTER-и tasks точно за §7.2; повторний запуск ідемпотентний
- [ ] Інжест фікстурного workspace дає очікувані лічильники задач/фаз/артефактів; бита картка не валить сканер
- [ ] logs/sessions.md → task_sessions(explicit); cwd+вікно → heuristic з confidence
- [ ] Окремий PR у plugins/core: SessionStart-hook + semver bump + CHANGELOG; hook пише лише при $AGENT_TASK_ID
- [ ] GET /api/tasks/{id}/artifacts/{artifact_id} з `../../` у будь-якому вигляді → 404; markdown проходить redaction
- [ ] Екран Workspaces: лічильники working/archive + 7-day $; вартість задачі = Σ сесій
- [ ] Heuristic-звʼязок пунктиром; confirm перемикає link_source='explicit' у БД, файли workspace незмінні (git status чистий)
- [ ] Live-інжест /Volumes/Work/swarmery-workspace: лічильники збігаються з диском; скріншоти прикладені
- [ ] Swarmery не створив/не змінив жодного файлу у workspace-репо

## Navigation

Previous: phase 3 — Agents registry · Parallel-safe with: phase 2 — Approvals ([phase-2-approvals/00-phase-2-plan.md](phase-2-approvals/00-phase-2-plan.md)) · Index: [00-plan.md](00-plan.md)

### Completion Report

```
(заповнюється виконавцем після завершення)
```
