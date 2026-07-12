# Step 2.2 — QUALITY GATE: hooks contract review + freeze

## Header

| Field | Value |
|---|---|
| Phase | 2 — Approvals + hooks |
| Duration | 2–3 h (human review + one short agent session for the freeze commit) |
| Type | GATE (human, critical) + agent-assisted freeze commit |
| Risk | Gate — protects both parallel branches from building on wrong hook semantics |
| Dependencies | Step 2.1 complete (`docs/hooks-format.md` committed) |

## Goal

Human validates the spike findings and freezes every cross-branch contract:
`types.ts` additions, `docs/ws-protocol.md` additions, the new
`docs/hooks-protocol.md` (HTTP contract the hook shim and the daemon share), and
the migration `0006` text. After this gate, agents A and B work in parallel and
may not change any frozen shape (requests go to `web/CONTRACT-REQUESTS.md`).

## Gate checklist (human)

- [ ] `docs/hooks-format.md` verdict: D1 holds (PermissionRequest fires only for would-prompt calls — E8 green) — or the documented fallback is chosen and this plan's steps 2.3/2.4 prompts are amended accordingly
- [ ] D3 holds: E5/E6 show no-output/timeout → native dialog appears, session survives
- [ ] E4 answer reviewed → set the default `approval_timeout` (Q-A) and record it in `docs/hooks-protocol.md`
- [ ] Owner sign-off on D2 (settings.local.json placement) and D4 (localhost, no auth) — these are policy, not code
- [ ] Q-B/Q-C decided or explicitly deferred (recorded in 00-phase-2-plan.md)
- [ ] Freeze commit reviewed and merged to main (see below)

## Agent Prompt (freeze commit)

```
Reference: docs/plan/phase-2-approvals/step-2.2-quality-gate-contract-freeze.md

Context:
Swarmery, гілка main. Спайк docs/hooks-format.md прийнято людиною.
Прочитай його розділ "Contract for phase 2", swarmery-design.md §2
(permission_requests), web/src/api/types.ts (FROZEN contract MVP),
docs/ws-protocol.md, internal/store/migrations/. Це docs+types-only
коміт — жодної Go/React-логіки.

Tasks:
1. web/src/api/types.ts — додай (нічого не змінюючи в наявному):
   export type PermissionRequestStatus =
     'pending'|'approved'|'denied'|'expired'|'resolved_elsewhere';
   export type ResolvedVia = 'dashboard'|'terminal'|'mobile';
   export interface PermissionRequest {
     id: number; sessionId: number; projectSlug: string;
     sessionTitle: string | null; toolName: string;
     requestJson: unknown;            // повний stdin хука (tool_input, permission_suggestions…)
     status: PermissionRequestStatus; requestedAt: string;
     expiresAt: string; resolvedAt: string | null;
     resolvedVia: ResolvedVia | null; reason: string | null;
   }
   /** GET /api/approvals?status=&limit= */
   export type ApprovalsResponse = PermissionRequest[];
   WSMessageType += 'permission_requested' | 'permission_resolved';
   WSMessage += { type:'permission_requested'; payload: PermissionRequest }
              | { type:'permission_resolved';  payload: PermissionRequest };
   HealthResponse += hooks_last_seen?: string | null;  // optional, additive
2. docs/ws-protocol.md — задокументуй два нові повідомлення (джерело
   емісії, приклади JSON, семантика: resolved емітиться і для expired).
3. docs/hooks-protocol.md (новий) — HTTP-контракт демона для хуків:
   POST /api/hooks/permission-request
     body: сирий stdin хука (pass-through JSON);
     200 {"decision":"allow"|"deny"|"none","reason":string|null,
          "requestId":number}   // "none" → шим виходить 0 без stdout
     long-poll до approval_timeout (дефолт — значення з гейту, E4/Q-A);
     429 → шим виходить 0 без stdout (rate limit);
   POST /api/hooks/stop — body: сирий stdin; завжди 202 (фаза 2 — лише
     heartbeat; канал для фази 2.5);
   GET /api/approvals?status=pending|resolved&limit=N → ApprovalsResponse;
   POST /api/approvals/{id} {"action":"approve"|"deny","reason"?:string}
     → 200 PermissionRequest | 409 якщо вже resolved;
   помилки — {"error":string}, як у наявному API. Плюс: семантика
   дедуплікації (SHA-256(session_uuid|tool_name|canonical tool_input);
   ідентичний pending → attach до наявного запиту), client-disconnect →
   resolved_elsewhere/via=terminal (згідно з E4), Origin-перевірка (D4),
   формат виводу шима (hookSpecificOutput за docs/hooks-format.md).
4. Текст міграції internal/store/migrations/0006_approvals.sql —
   ТІЛЬКИ додавання: ALTER TABLE permission_requests ADD COLUMN
   dedup_hash TEXT; ADD COLUMN expires_at TEXT; ADD COLUMN reason TEXT;
   CREATE INDEX idx_pr_dedup ON permission_requests(session_id,
   dedup_hash, status). Файл комітиться зараз (Agent A його підключить),
   існуючі 0001–0005 не чіпати.
5. npm run build (типи компілюються), bash -n не потрібен; go vet
   зелений (міграція — .sql, Go не змінюється).

Boundaries:
- Наявні імена/поля в types.ts, ws-protocol.md, міграціях 0001–0005 —
  недоторканні (MVP-контракт byte-identical).
- Жодної імплементації — тільки типи, docs, .sql.

Output / Validation:
Один conventional commit "docs(swarmery): phase 2 contract freeze —
approvals HTTP/WS/types + migration 0006" на main. Заповни Completion
Report у step-2.2-quality-gate-contract-freeze.md.
```

## Success Criteria

- [ ] Freeze commit on main: types.ts additions compile (`npm run build`), MVP names untouched (`git diff` shows additions only)
- [ ] `docs/hooks-protocol.md` fully determines both sides of the HTTP contract (a stranger could implement either end)
- [ ] Migration 0006 text is additive-only and committed
- [ ] All gate checkboxes above ticked by the human — a red box stops the phase

## Navigation

Previous: [step-2.1-hooks-spike.md](step-2.1-hooks-spike.md) · Next: [step-2.3-agent-a-hooks-backend.md](step-2.3-agent-a-hooks-backend.md) + [step-2.4-agent-b-approvals-ui.md](step-2.4-agent-b-approvals-ui.md) (parallel) · Index: [00-phase-2-plan.md](00-phase-2-plan.md)

### Completion Report

```
(заповнюється виконавцем після завершення)
```
