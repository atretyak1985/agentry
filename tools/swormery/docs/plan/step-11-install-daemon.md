# Step 11 — T3.5: `swormery install` (launchd auto-start)

## Header

| Field | Value |
|---|---|
| Phase | 4 — Integration, install, ship |
| Duration | 1 short agent session, ~1–2 h (MEDIUM confidence) |
| Type | Agent session (code) |
| Risk | Low — isolated to cmd/ + new package; reversible via uninstall |
| Dependencies | Step 10 |

## Goal

Make the daemon always-on: `swormery install` registers a launchd agent so the
dashboard survives reboots; `uninstall` removes it cleanly; `status` reports health.

## Automation

Fresh Claude Code session in `/Volumes/Work/swarmery/tools/swormery`.

## Agent Prompt

```
Reference: docs/plan/step-11-install-daemon.md

Context:
Репозиторій Swormery, main після інтеграції (step 10), make build працює.
Прочитай cmd/swormery. Задача: автозапуск демона на macOS через launchd,
щоб після логіна дашборд завжди був живий на localhost:7777.

Tasks:
1. swormery install:
   - копіює поточний бінарник у ~/.swormery/bin/swormery
   - пише ~/Library/LaunchAgents/com.swormery.daemon.plist:
     ProgramArguments = [~/.swormery/bin/swormery, serve],
     RunAtLoad=true, KeepAlive=true,
     StandardOutPath/StandardErrorPath → ~/.swormery/logs/
   - launchctl bootstrap gui/$(id -u) <plist> (сучасний API, не load)
   - ІДЕМПОТЕНТНІСТЬ: повторний install = оновити бінарник і
     перезапустити сервіс (bootout → bootstrap), нічого не дублювати
2. swormery uninstall: bootout + видалити plist (логи і БД лишити)
3. swormery status: чи запущено, версія, PID, аптайм, розмір БД
4. НЕ чіпай hooks у ~/.claude/settings.json — це Фаза 2.

Boundaries:
- Тільки cmd/ і новий internal/installer. Ніякого UI.
- Тести: генерація plist (golden file) та ідемпотентність
  (повторний install не додає другий сервіс) — logic-рівень, без
  реального launchctl у тестах.

Output / Validation:
go test зелені. Живий тест: make build && ./swormery install →
launchctl print gui/$(id -u)/com.swormery.daemon показує running →
curl -s localhost:7777/api/stats/today відповідає → swormery uninstall
чисто прибирає. Покажи вивід кожного кроку. Conventional commit.
Заповни Completion Report у docs/plan/step-11-install-daemon.md.
```

## Detailed Instructions

- Port: plist must respect `SWORMERY_PORT` if the user configured one — write it into
  `EnvironmentVariables` in the plist when the flag/env was set at install time.
- Watch out for a port clash during the live test: stop any manually-running
  `./swormery serve` before `install`.
- Reversibility: `uninstall` is the rollback; DB and logs are intentionally preserved.

## Success Criteria

- [ ] `go test` green incl. plist golden-file + idempotency tests
- [ ] `install` → `launchctl print` shows the service running; API answers on :7777
- [ ] Second `install` run leaves exactly one service registered (idempotent)
- [ ] `uninstall` removes plist + service; `~/.swormery/{swormery.db,logs}` remain
- [ ] `status` prints running-state, PID, version, DB size
- [ ] `~/.claude/settings.json` untouched

## Navigation

Previous: [step-10-integration.md](step-10-integration.md) · Next: [step-12-quality-gate-ship-dogfood.md](step-12-quality-gate-ship-dogfood.md) · Index: [00-plan.md](00-plan.md)

### Completion Report

```
Date/agent: · Commit SHA: · Live-test output summary: · Notes:
```
