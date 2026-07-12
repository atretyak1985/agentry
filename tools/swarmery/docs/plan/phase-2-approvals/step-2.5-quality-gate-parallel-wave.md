# Step 2.5 — QUALITY GATE: parallel-wave verification

## Header

| Field | Value |
|---|---|
| Phase | 2 — Approvals + hooks |
| Duration | 1–2 h (human) |
| Type | GATE (human) |
| Risk | Gate — protects main from a broken/drifted merge |
| Dependencies | Steps 2.3 and 2.4 report complete (Completion Reports filled in both worktrees) |

## Goal

Verify both branches independently before integration: green checks, frozen-contract
compliance, reviewed UX. Same procedure as MVP Gate 09.

## Gate checklist (human)

Backend (`feat/swarmery-hooks`, worktree `/Volumes/Work/swarmery-wt-hooks`):

- [ ] `go vet ./...` + `go test -race ./...` green locally
- [ ] Smoke replayed by hand: `serve` → `hooks install` into a scratch project → curl long-poll → curl approve → shim JSON correct
- [ ] Shim fail-open demonstrated live: daemon stopped, hooked `claude` session's dialog appears normally, added latency subjectively imperceptible (< 1.5 s)
- [ ] `hooks install` run twice on the same project → "already installed", `settings.local.json` unchanged (`git diff` / `diff` vs copy)
- [ ] Diff scope matches step 2.3 success criteria (no `web/**`, no frozen-file edits)
- [ ] `web/CONTRACT-REQUESTS.md` — new entries reviewed; none silently implemented

Frontend (`feat/swarmery-approvals-ui`, worktree `/Volumes/Work/swarmery-wt-approvals-ui`):

- [ ] `npm run build` green; zero new npm dependencies (`git diff package.json`)
- [ ] Screenshots reviewed: Approvals mobile + desktop match the design language (dark editorial, amber accents for pending)
- [ ] Mock walkthrough in the browser: badge live-updates, Approve/Deny, history chips, session links, Overview widget
- [ ] Diff scope matches step 2.4 success criteria (`web/**` only, `types.ts` untouched)

Cross-cutting:

- [ ] Both sides implemented `docs/hooks-protocol.md` literally — spot-check 3 shapes (long-poll response, approvals list item, WS `permission_resolved`) against the doc
- [ ] Merge order confirmed: backend → frontend (step 2.6); no rebase surprises (`git log main..` both branches)

## Failure handling

Any red box → the owning agent fixes on its branch and this gate re-runs. Contract
disputes are resolved by editing `docs/hooks-protocol.md`/`types.ts` on main first
(a new mini-freeze commit), then rebasing both branches — never by diverging
implementations.

## Navigation

Previous: [step-2.4-agent-b-approvals-ui.md](step-2.4-agent-b-approvals-ui.md) · Next: [step-2.6-integration-live-test.md](step-2.6-integration-live-test.md) · Index: [00-phase-2-plan.md](00-phase-2-plan.md)

### Completion Report

```
(заповнюється людиною після проходження гейту: дата, вердикт по кожному блоку, знайдені розбіжності)
```
