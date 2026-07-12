# Step 08 — Agent C: cost & today-stats

## Header

| Field | Value |
|---|---|
| Phase | 3 — Parallel wave |
| Duration | 1 agent session, ~2–3 h (MEDIUM confidence — smallest of the wave) |
| Type | Agent session (code, runs in parallel with steps 06, 07) |
| Risk | Low-Medium — wrong pricing silently corrupts $ totals |
| Dependencies | Step 05 gate PASS; worktree `/Volumes/Work/swarmery-wt-metrics` (work in `tools/swarmery`) |

## Goal

Per-turn USD cost from usage fields + `config/pricing.json` (web-verified prices),
the frozen `GET /api/stats/today` endpoint, and a `recost` backfill command.
Honesty rule: unknown model → cost NULL, never 0.

## Automation

Fresh Claude Code session in `/Volumes/Work/swarmery-wt-metrics/tools/swarmery` (worktree, branch
`feat/swarmery-metrics`). Needs web search for pricing verification.

## Agent Prompt

```
Reference: docs/plan/step-08-agent-c-metrics.md

Context:
Репозиторій Swarmery після T1. Гілка feat/swarmery-metrics (worktree). Прочитай
swarmery-design.md (розділи 1-2), docs/jsonl-format.md (секція про usage),
internal/store. НЕ чіпай web/ і internal/ingest (крім хука нижче) —
паралельні агенти. У internal/api — тільки блок "// wave C: stats"
у routes.go.

Tasks:
1. config/pricing.json: ціни моделей (input/output/cache_read/cache_write
   за 1M токенів). Заповни актуальними цінами Claude-моделей — ПЕРЕВІР
   через web search на platform.claude.com, НЕ з памʼяті. Читання на
   старті, hot-reload не потрібен.
2. internal/cost: розрахунок cost_usd для turn з usage-полів + назви
   моделі; невідома модель → cost NULL + warn (не 0, щоб не брехати
   в сумах).
3. Інтеграційна точка: чиста функція EnrichTurn(turn) — виклич її з
   ОДНОГО місця в ingest (мінімальний дотик, познач коментарем
   // metrics hook для мерджу).
4. API: GET /api/stats/today?project= — відповідь СТРОГО за типом
   StatsToday з web/src/api/types.ts (заморожено):
   {sessions, active, tokens_in, tokens_out, cost_usd, errors} —
   агрегат по events/turns за сьогодні (локальна TZ). Без rollup-таблиць —
   прямий запит, на MVP-обсягах ок.
5. Backfill-команда: swarmery recost — перерахунок cost_usd для всіх turns
   (на випадок зміни pricing.json); попереджай, якщо демон запущений
   (одночасний запис у WAL).

Boundaries:
- НЕ створюй daily_rollups логіку (Фаза 6). НЕ чіпай web/ і types.ts.
- Зміни в internal/ingest — тільки один виклик EnrichTurn.
- Жодних нових зовнішніх залежностей.

Output / Validation:
go test: табличні кейси розрахунку (з cache-токенами; невідома модель →
NULL; нульовий usage). curl /api/stats/today повертає адекватні числа на
заінджещених fixtures. Conventional commits у feat/swarmery-metrics. Заповни
Completion Report у docs/plan/step-08-agent-c-metrics.md (worktree).
```

## Detailed Instructions

- Cost formula per turn: `(in/1e6)*p.input + (out/1e6)*p.output +
  (cache_read/1e6)*p.cache_read + (cache_write/1e6)*p.cache_write`; round only at
  display time, store full float.
- `pricing.json` keys must match model names as they appear in JSONL (see
  `docs/jsonl-format.md`); include a `fallback_prefixes` map if JSONL uses versioned
  ids (e.g., prefix-match `claude-sonnet-…`).
- SUM over NULL costs: `cost_usd` in StatsToday is NULL only if **all** turns are
  unpriced; otherwise sum priced turns and log how many were skipped.

## Success Criteria

- [ ] `go test ./internal/cost/...` green with table-driven cases incl. cache tokens and unknown-model→NULL
- [ ] `pricing.json` values match platform.claude.com at implementation time (cite URL in commit body)
- [ ] `curl ':7777/api/stats/today'` on fixtures returns numbers consistent with fixture usage sums
- [ ] `swarmery recost` recomputes all turns idempotently
- [ ] Diff touches only `internal/cost`, `config/`, one `EnrichTurn` call in ingest, routes.go wave-C block, `cmd/`

## Navigation

Previous: [step-07-agent-b-frontend.md](step-07-agent-b-frontend.md) (parallel) · Next: [step-09-quality-gate-parallel-wave.md](step-09-quality-gate-parallel-wave.md) · Index: [00-plan.md](00-plan.md)

### Completion Report

```
Date/agent: · Branch head SHA: · Pricing source URL: · Models priced: · CONTRACT-REQUESTS entries:
```
