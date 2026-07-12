# Step 2.7 — QUALITY GATE: ship & dogfood

## Header

| Field | Value |
|---|---|
| Phase | 2 — Approvals + hooks |
| Duration | 1–2 h + passive dogfooding over the following days |
| Type | GATE (human, final) |
| Risk | Gate — the phase is not "done" until it survives real daily use |
| Dependencies | Step 2.6 complete (P1–P10 green on the launchd daemon) |

## Goal

Ship decision: hooks go live on real projects, rollback path is proven, phase 2.5
(Reporter) is unblocked, and dogfooding capture is armed.

## Gate checklist (human)

Ship readiness:

- [ ] All success criteria in [00-phase-2-plan.md](00-phase-2-plan.md) ticked with evidence (Completion Reports 2.1–2.6)
- [ ] `swarmery install` daemon running the phase-2 binary; `/api/health` shows `hooks_last_seen`
- [ ] `swarmery hooks install --all` executed consciously (owner decision: all projects vs a starter subset — record which)
- [ ] One real (non-scratch) approval performed from the dashboard on a live working session

Rollback drill (performed, not assumed):

- [ ] `swarmery hooks uninstall --all` → zero `swarmery hook` occurrences under any project's `.claude/` (verified by grep); re-install afterwards if shipping
- [ ] Daemon rollback story stated: `git revert` of the two merge commits + `make build` + `swarmery install`; migration 0006 columns are additive and safe to leave in place
- [ ] Kill-switch understood: uninstalling hooks alone fully restores pre-phase-2 behavior even with the new daemon running

Downstream readiness:

- [ ] Stop-hook channel verified (2.6 P9) — [../phase-2.5-reporter-agent-d.md](../phase-2.5-reporter-agent-d.md) dependency "Після фази 2" is now satisfiable
- [ ] Open questions Q-A–Q-D re-reviewed with live-protocol data; decisions or deferrals recorded in 00-phase-2-plan.md
- [ ] `phase2-backlog.md` "Phase 2 dogfooding" section armed (same capture discipline as T4: `- [екран] чого не вистачало — чому важливо`)
- [ ] Progress checklist in 00-phase-2-plan.md fully ticked; roadmap line in [../00-plan.md](../00-plan.md) marked shipped with the date

Watch items for the dogfooding window (add to backlog as observed):

- Approval fatigue: if the dashboard queue mirrors too many trivial dialogs, revisit Q-B ("always allow" writing permissions rules back)
- Claude Code version drift: any hook contract change → update `docs/hooks-format.md` first (same ratchet as `jsonl-format.md`)
- `wait_minutes` (design §2 `daily_rollups`) now has real source data — candidate for the Analytics phase

## Navigation

Previous: [step-2.6-integration-live-test.md](step-2.6-integration-live-test.md) · Index: [00-phase-2-plan.md](00-phase-2-plan.md) · Next phase: [../phase-2.5-reporter-agent-d.md](../phase-2.5-reporter-agent-d.md)

### Completion Report

```
(заповнюється людиною: дата шипу, скоуп hooks install, результат rollback-drill, рішення по Q-A–Q-D)
```
