# Step 10 — T3: integration (merge A+B+C into main)

## Header

| Field | Value |
|---|---|
| Phase | 4 — Integration, install, ship |
| Duration | 1 agent session, ~2–4 h (MEDIUM confidence — conflicts pre-limited by routes.go blocks) |
| Type | Agent session (merge + glue, no new features) |
| Risk | Medium — contract mismatches surface here |
| Dependencies | Step 09 gate PASS |

## Goal

Merge the three branches into `main` in a fixed order, resolve contract requests,
connect the frontend to the real API/WS, and validate the whole system on this
machine's real history.

## Automation

Fresh Claude Code session in `/Volumes/Work/swarmery/tools/swormery` (main checkout, not a worktree).

## Agent Prompt

```
Reference: docs/plan/step-10-integration.md

Context:
Репозиторій Swormery, гілки feat/swormery-ingest, feat/swormery-frontend, feat/swormery-metrics готові
і пройшли gate (step 09). Прочитай зведений список contract-requests із
docs/plan/step-09-quality-gate-parallel-wave.md (Completion Report),
web/CONTRACT-REQUESTS.md з гілок, docs/ws-protocol.md, і дифи всіх трьох
гілок відносно main.

Tasks:
1. Змердж гілки в main у порядку: ingest → metrics → frontend, розвʼяжи
   конфлікти (routes.go має окремі блоки wave A/C — конфліктів бути
   майже не повинно). Розбіжності з CONTRACT-REQUESTS — реалізуй на
   бекенді або задокументуй у файлі, чому ні.
2. Прибери mock-режим за замовчуванням (VITE_MOCK=1 лишається опцією),
   зʼєднай фронтенд з реальним API і WS. Онови types.ts з фінальних
   Go-структур — це ЄДИНЕ місце, де types.ts можна змінювати.
3. make build → один бінарник з embedded фронтендом.
4. Наскрізний прогін: повний backfill реальної історії цієї машини,
   перевір Overview і 3-4 session details різних проєктів
   (swarmery, bloomblum, Skygor) — таймлайн, субагенти, дифи, вартість.

Boundaries:
- Ніяких нових фіч. Тільки мердж, склейка, фікси інтеграційних багів.
- Кожен мердж — окремий merge-коміт (conventional), CI зелений після кожного.

Output / Validation:
go test + npm run build зелені в main. ./swormery serve на реальних даних:
підсумок — скільки проєктів/сесій/подій заінджестено, сумарна вартість за
сьогодні, URL. Live-тест: нова сесія claude зʼявляється в Overview без
перезавантаження сторінки. Заповни Completion Report у
docs/plan/step-10-integration.md.
```

## Detailed Instructions

- Merge order rationale: ingest brings WS + schema services (foundation), metrics
  hooks into ingest (`EnrichTurn`), frontend consumes both — foundation → wire →
  consume.
- After merges, clean up:
  ```bash
  cd /Volumes/Work/swarmery/tools/swormery
  git -C /Volumes/Work/swarmery worktree remove ../swarmery-wt-ingest ../swarmery-wt-frontend ../swarmery-wt-metrics
  git branch -d feat/swormery-ingest feat/swormery-frontend feat/swormery-metrics
  ```
- Rollback: each branch lands as its own merge commit — `git revert -m 1 <merge-sha>`
  in reverse order (frontend → metrics → ingest) restores the pre-integration state;
  DB schema was untouched (additive rule), so no migration rollback is needed.

## Success Criteria

- [ ] `main` contains all three merges; CI green; worktrees removed
- [ ] `make build` single binary; dashboard serves embedded SPA (no Vite dev server)
- [ ] Full real backfill succeeds; Overview shows today's tokens + cost
- [ ] 3–4 real session details verified (timeline, nested subagents, diffs, cost)
- [ ] Live test: new `claude` session visible in Overview < 3 s, no reload
- [ ] Every CONTRACT-REQUESTS entry implemented or answered in the file

## Navigation

Previous: [step-09-quality-gate-parallel-wave.md](step-09-quality-gate-parallel-wave.md) · Next: [step-11-install-daemon.md](step-11-install-daemon.md) · Index: [00-plan.md](00-plan.md)

### Completion Report

```
Date/agent: · Merge SHAs: · Backfill stats (projects/sessions/events): · Today cost: · Integration bugs fixed:
```
