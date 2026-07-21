# Orchestration Plan — 2026-07-01-full-card

## Triage & mode

TRIAGE | mode: Sprint | score: 6 | rationale: two repos, moderate risk.

## Planned subagents

| agent | phase | purpose | parallel group | expected artifact |
|---|---|---|---|---|
| @core:context-gatherer | 2 | map the touched modules | A | phases/02-context.md |
| @core:implementation-agent | 5 | build the endpoint | B | src changes |

## Verification strategy

- `make test` must pass; ACCEPT on green, RE-DISPATCH on any failure.

## Loop 1 — corrected instructions
- Failed: go test ./internal/api — TestRetroAgents wanted 2 agents, got 1
- Brief delta: added the missing subagent_start fixture row to the brief
- Why this succeeds now: the fixture now covers both notations.

## Loop 2 — corrected instructions
- Failed: tsc --noEmit — RetroAgentRow missing re_dispatch_rate
- Brief delta: extend types.ts before the component edit
