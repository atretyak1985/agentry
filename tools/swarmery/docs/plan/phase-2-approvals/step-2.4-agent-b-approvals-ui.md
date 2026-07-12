# Step 2.4 — Agent B: Approvals screen (frontend, frozen contract)

## Header

| Field | Value |
|---|---|
| Phase | 2 — Approvals + hooks (parallel wave) |
| Duration | 1 agent session, ~3–4 h (MEDIUM — analogous to MVP step 07, smaller surface) |
| Type | Agent session (code, runs in parallel with step 2.3) |
| Risk | Medium — pure consumer of the frozen contract; works against mocks until integration |
| Dependencies | Gate 2.2 PASS; worktree `/Volumes/Work/swarmery-wt-approvals-ui` (work in `tools/swarmery`), branch `feat/swarmery-approvals-ui` |

## Goal

Approvals screen per design §3.2: pending list (tool, collapsed `request_json`
summary, session link, live age), Approve/Deny actions, decision history (audit:
`resolved_via`/when), nav badge with the pending count, `waiting_approval` made
visible everywhere sessions render, Overview pending widget (design §3.1).

## Automation

Fresh Claude Code session in `/Volumes/Work/swarmery-wt-approvals-ui/tools/swarmery`
(worktree, branch `feat/swarmery-approvals-ui`). Backend endpoints do not exist yet —
develop against extended mocks (`web/src/mock/`), same pattern as MVP step 07.

## Agent Prompt

```
Reference: docs/plan/phase-2-approvals/step-2.4-agent-b-approvals-ui.md

Context:
Swarmery SPA після MVP (React 18 + TS strict + Vite + Tailwind, дизайн-
мова — темна editorial, див. web/src/pages/* і swarmery-ui-mockup.html).
Контракт заморожено: web/src/api/types.ts вже містить PermissionRequest,
ApprovalsResponse, WS-типи permission_requested|permission_resolved;
HTTP — docs/hooks-protocol.md. Прочитай їх, web/src/lib/ws.ts,
web/src/mock/{data,ws}.ts, App.tsx (nav), swarmery-design.md §3.1–3.2.
Працюєш у гілці feat/swarmery-approvals-ui (worktree). Паралельно
Agent A робить бекенд — НЕ чіпай internal/**, cmd/**, migrations,
types.ts (запити на зміну контракту → web/CONTRACT-REQUESTS.md).

Tasks:
1. web/src/pages/Approvals.tsx (роут /approvals): секція Pending —
   картка: іконка/назва tool, розумний summary з requestJson
   (Bash → command; Edit|Write → file_path; MCP → імʼя інструмента;
   решта — перший рядок JSON), розгортання повного JSON (<pre>,
   як payload у таймлайні), лінк на сесію (/sessions/{sessionId},
   projectSlug + sessionTitle), живий вік ("висить 1м 23с", тікер 1с),
   бейдж часу до expiresAt. Кнопки Approve / Deny (Deny — з опційним
   reason у popover) → POST /api/approvals/{id}, optimistic-переніс у
   History, 409 → тихий refetch. Секція History (status=resolved,
   limit 50): рішення, resolved_via як chip (dashboard|terminal|mobile),
   відносний час.
2. Nav: пункт Approvals у App.tsx (bottom bar / sidebar) з amber-бейджем
   pending-каунту (як "Approvals (3)" у design §3.2); каунт живе через
   WS: permission_requested → +1, permission_resolved → −1, ресинк
   через GET /api/approvals?status=pending на mount/reconnect (WS —
   hint stream, не джерело істини, див. docs/ws-protocol.md).
3. WS-інтеграція: розшир lib/ws.ts обробкою двох нових типів (naming
   строго з types.ts); Approvals-екран оновлюється без reload; порожній
   стан — "No pending approvals" у стилі наявних empty-states.
4. waiting_approval видимість: SessionCard/ui.tsx вже мають amber-стилі
   для waiting_approval — перевір, що бейдж/фільтр статусу на Sessions
   його показують; Overview: віджет "Pending approvals: топ-3 + кнопка
   в чергу" (design §3.1) — компактний список над сесіями; поле
   waiting_approval у StatsOverview вже в контракті.
5. Моки: mock/data.ts — фікстури PermissionRequest (pending різного
   віку + resolved усіх статусів, включно expired/resolved_elsewhere);
   mock/ws.ts — сценарій: permission_requested через 3с, resolved через
   10с (для ручної перевірки бейджа).
6. Мобільний: список/картки адаптивні як решта екранів; swipe-дії
   (approve вправо / deny вліво) — SLC-бонус, тільки якщо без нових
   залежностей (Q-C: стретч, не блокер).
7. Скріншоти: web/screenshots/approvals.png + approvals-desktop.png
   через наявний scripts/screenshot.mjs (мок-режим).

Boundaries:
- НЕ чіпай internal/**, cmd/**, migrations/**, web/src/api/types.ts.
- Нові npm-залежності: 0.
- TS strict, без any; DRY з наявними компонентами (ui.tsx, format.ts).

Output / Validation:
npm run build + наявні перевірки зелені; скріншоти додано. Conventional
commits у feat/swarmery-approvals-ui. Заповни Completion Report у
docs/plan/phase-2-approvals/step-2.4-agent-b-approvals-ui.md (у worktree).
```

## Detailed Instructions

- Age/expiry tickers: one shared 1 s interval for the page, not per-card timers.
- Optimistic resolve must reconcile with the авторитетним WS `permission_resolved`
  (idempotent upsert by `id` — the same message will arrive for your own action).
- The badge count lives in the app shell (App.tsx) — lift the WS subscription or
  reuse the existing shared WS connection from `lib/ws.ts`; do not open a second
  socket.
- `requestJson` is `unknown` — narrow via type guards (no `as any`); malformed
  payload renders as raw JSON, never crashes the card.

## Success Criteria

- [ ] `npm run build` green (TS strict); zero new dependencies
- [ ] Mock scenario: badge appears within 3 s, decrements on resolve, survives reload (REST resync)
- [ ] Approve/Deny flows work against mocks incl. 409 path; Deny records a reason
- [ ] Pending card: tool summary, expandable JSON, session link, live age, expiry countdown
- [ ] History shows `resolved_via` chips for all five statuses; Overview shows the top-3 pending widget
- [ ] Screenshots committed; diff touches only `web/**` (except `web/src/api/types.ts`) and `docs/`

## Navigation

Previous: [step-2.3-agent-a-hooks-backend.md](step-2.3-agent-a-hooks-backend.md) (parallel) · Next: [step-2.5-quality-gate-parallel-wave.md](step-2.5-quality-gate-parallel-wave.md) · Index: [00-phase-2-plan.md](00-phase-2-plan.md)

### Completion Report

```
(заповнюється виконавцем після завершення)
```
