# Swarmery — Data Model & UI Structure

Дизайн-документ для control plane агентної системи (Claude Code).
Мета: схема SQLite для MVP-спостереження, яка вже закладає майбутні шари —
approvals, чергу задач, менеджмент агентів/скілів, метрики якості та evals.

---

## 1. Принципи схеми

1. **Append-only event log як ядро.** Все, що відбувається в сесіях, — це події.
   Таблиця `events` з типізованим `payload` (JSON) — толерантна до змін формату
   JSONL Claude Code: незнайомі поля просто лягають у payload, парсер не падає.
2. **Атрибуція через nullable FK.** Кожна подія може посилатись на `agent_id`
   та `skill_id`. Метрики по агентах — це просто агрегати по events, без
   окремої "аналітичної" схеми на старті.
3. **Версіонування через content hash + git.** Агенти і скіли — файли на диску.
   Кожна зміна фіксується як версія (hash вмісту, опційно git SHA). Це дає
   rollback і A/B порівняння промптів пізніше — без зміни схеми.
4. **Rollup-таблиці замість важких запитів.** Дашборд не сканує мільйони подій:
   демон раз на N хвилин оновлює `daily_rollups`.
5. **Два джерела, одна дедуплікація.** Події приходять з hooks (live) і з
   парсера JSONL (backfill). Ключ дедуплікації: `uuid` запису, якщо він є;
   для рядків без uuid — SHA-256 від (шлях файлу + вміст рядка). Див. комент
   до `events.dedup_key`.

---

## 2. SQLite схема (DDL)

```sql
PRAGMA journal_mode = WAL;
PRAGMA foreign_keys = ON;

-- ============ Реєстр проєктів і сесій ============

CREATE TABLE projects (
    id            INTEGER PRIMARY KEY,
    path          TEXT NOT NULL UNIQUE,      -- /Volumes/Work/bloomblum
    slug          TEXT NOT NULL,             -- як у ~/.claude/projects/
    name          TEXT,                      -- людська назва (редагується в UI)
    first_seen    TEXT NOT NULL,             -- ISO 8601
    last_activity TEXT,
    archived      INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE sessions (
    id           INTEGER PRIMARY KEY,
    project_id   INTEGER NOT NULL REFERENCES projects(id),
    session_uuid TEXT NOT NULL UNIQUE,       -- з імені JSONL-файлу
    parent_uuid  TEXT,                       -- resume/fork ланцюжки; у форматі JSONL
                 -- джерела не спостерігалось (C4) — у MVP лишається NULL
    model        TEXT,                       -- claude-fable-5 ...
    git_branch   TEXT,
    cwd          TEXT,
    status       TEXT NOT NULL DEFAULT 'active',
                 -- active | waiting_approval | idle | completed | killed
                 -- MVP обчислює лише active|idle|completed евристикою (C5):
                 -- mtime файлу / фінальний system:turn_duration;
                 -- waiting_approval|killed зарезервовані для hooks (Phase 2)
    started_at   TEXT NOT NULL,
    ended_at     TEXT,                       -- nullable; = timestamp останнього рядка,
                 -- щойно сесія стала неактивною (session_end запису не існує)
    title        TEXT,                       -- перший промпт, обрізаний
    source       TEXT NOT NULL DEFAULT 'jsonl'  -- jsonl | hook | both
);
CREATE INDEX idx_sessions_project ON sessions(project_id, started_at DESC);
CREATE INDEX idx_sessions_status  ON sessions(status);

-- ============ Turns і Events (ядро) ============

CREATE TABLE turns (
    id          INTEGER PRIMARY KEY,
    session_id  INTEGER NOT NULL REFERENCES sessions(id),
    seq         INTEGER NOT NULL,            -- порядок у сесії; НЕ user/assistant
                -- чергування: один user-промпт породжує N assistant API-повідомлень (C2);
                -- seq виводиться групуванням записів за promptId (fallback: правило
                -- user-message-opener з docs/jsonl-format.md)
    role        TEXT NOT NULL,               -- user | assistant
    message_id  TEXT,                        -- API message.id (assistant-turns) —
                -- ключ дедуплікації usage / ідемпотентності; NULL для user-turns
    started_at  TEXT NOT NULL,
    ended_at    TEXT,
    -- usage дублюється дослівно на кожному з N JSONL-рядків однієї API-відповіді (C1):
    -- перед агрегацією токени ОБОВ'ЯЗКОВО дедуплікувати за message.id;
    -- user-turns не мають usage — колонки лишаються NULL
    tokens_in   INTEGER,
    tokens_out  INTEGER,
    tokens_cache_read  INTEGER,
    tokens_cache_write INTEGER,
    cost_usd    REAL,                        -- розрахунок по прайсу моделі
    UNIQUE(session_id, seq)
);

CREATE TABLE events (
    id          INTEGER PRIMARY KEY,
    session_id  INTEGER NOT NULL REFERENCES sessions(id),
    turn_id     INTEGER REFERENCES turns(id),
    ts          TEXT NOT NULL,
    type        TEXT NOT NULL,
        -- tool_call | subagent_start | subagent_stop | skill_use
        -- | file_change | permission_request | permission_resolved
        -- | error | test_run | commit | user_prompt | session_end
        -- subagent_start/stop не існують як JSONL-записи — виводяться з
        -- tool_use name="Agent" та його matching tool_result (C6);
        -- sidechain-транскрипти лежать у <sessionId>/subagents/agent-<id>.jsonl,
        -- join до батьківської події через meta.json toolUseId.
        -- NOT ingested у MVP (ignore / лише payload, нових типів не додаємо):
        -- pr-link, checkpoint-записи, system-субтипи крім
        -- api_error/turn_duration/compact_boundary, attachments
    tool_name   TEXT,                        -- Bash, Edit, Agent, Skill...
    agent_id    INTEGER REFERENCES agents(id),   -- nullable: FK-резолюція лише для
                -- project-local агентів; вбудовані subagent-типи (Explore,
                -- general-purpose) не є рядками реєстру — NULL,
                -- ім'я лишається в payload (agentType)
    skill_id    INTEGER REFERENCES skills(id),   -- nullable: plugin-скіли не є рядками
                -- реєстру — NULL, ім'я лишається в payload (attributionSkill)
    parent_event_id INTEGER REFERENCES events(id), -- дерево делегування
    status      TEXT,                        -- ok | error | denied | timeout
    duration_ms INTEGER,
    payload     TEXT,                        -- JSON: сирі деталі події
    dedup_key   TEXT UNIQUE                  -- `uuid` запису, якщо він є (C3);
                -- для рядків без uuid — SHA-256 від (шлях файлу + вміст рядка).
                -- Sidechain-файли мають власний простір uuid; ключ мусить бути
                -- глобально унікальним across main+sidechain файлів
);
CREATE INDEX idx_events_session ON events(session_id, ts);
CREATE INDEX idx_events_agent   ON events(agent_id, ts);
CREATE INDEX idx_events_type    ON events(type, ts);

-- ============ Зміни файлів (diff-стрічка, out-of-scope детектор) ============

CREATE TABLE file_changes (
    id           INTEGER PRIMARY KEY,
    event_id     INTEGER NOT NULL REFERENCES events(id),
    session_id   INTEGER NOT NULL REFERENCES sessions(id),
    file_path    TEXT NOT NULL,
    change_type  TEXT NOT NULL,              -- create | edit | delete | rename
    additions    INTEGER,
    deletions    INTEGER,
    diff         TEXT,                       -- unified diff (або ref на blob)
    out_of_scope INTEGER NOT NULL DEFAULT 0  -- поза заявленим scope задачі
);
CREATE INDEX idx_fc_session ON file_changes(session_id);
CREATE INDEX idx_fc_path    ON file_changes(file_path);

-- ============ Approvals (шар 2: втручання) ============

CREATE TABLE permission_requests (
    id           INTEGER PRIMARY KEY,
    session_id   INTEGER NOT NULL REFERENCES sessions(id),
    event_id     INTEGER REFERENCES events(id),
    tool_name    TEXT NOT NULL,
    request_json TEXT NOT NULL,              -- що саме агент хоче зробити
    status       TEXT NOT NULL DEFAULT 'pending',
                 -- pending | approved | denied | expired | resolved_elsewhere
    requested_at TEXT NOT NULL,
    resolved_at  TEXT,
    resolved_via TEXT                        -- dashboard | terminal | mobile
);
CREATE INDEX idx_pr_pending ON permission_requests(status, requested_at);

-- ============ Черга задач (шар 2: оркестрація) ============

CREATE TABLE tasks (
    id          INTEGER PRIMARY KEY,
    project_id  INTEGER NOT NULL REFERENCES projects(id),
    title       TEXT NOT NULL,
    prompt      TEXT NOT NULL,               -- spec-driven промпт
    priority    INTEGER NOT NULL DEFAULT 5,  -- 1 = найвищий
    status      TEXT NOT NULL DEFAULT 'queued',
                -- queued | running | needs_review | done | failed | cancelled
    session_id  INTEGER REFERENCES sessions(id),  -- яка сесія виконує/виконала
    agent_id    INTEGER REFERENCES agents(id),    -- цільовий агент (якщо є)
    created_at  TEXT NOT NULL,
    started_at  TEXT,
    finished_at TEXT,
    result_note TEXT,                        -- людська оцінка: ok / правив / відкат
    reverted    INTEGER NOT NULL DEFAULT 0   -- для quality-метрик
);
CREATE INDEX idx_tasks_queue ON tasks(status, priority, created_at);

-- ============ Наративи, звіти, чеклісти (фаза 2.5: Reporter) ============

CREATE TABLE narratives (
    id           INTEGER PRIMARY KEY,
    session_id   INTEGER NOT NULL UNIQUE REFERENCES sessions(id),
    summary_json TEXT NOT NULL,              -- {done[], decisions[{what,why}],
                 -- failures[], risks[], followups[]} — ТІЛЬКИ текст від LLM;
                 -- усі числа звіт бере з БД, числа з LLM ігноруються
    model        TEXT,                       -- дешева модель Reporter-а (з конфігу)
    tokens       INTEGER,                    -- вартість роботи самого Reporter-а —
    cost_usd     REAL,                       -- видна в Analytics
    created_at   TEXT NOT NULL,
    trigger      TEXT NOT NULL               -- auto_stop | manual | digest
);

CREATE TABLE reports (
    id          INTEGER PRIMARY KEY,
    kind        TEXT NOT NULL,               -- session | task | weekly_digest | incident
    ref_id      INTEGER,                     -- session_id або task_id (за kind);
                -- NULL для weekly_digest
    period      TEXT,                        -- ISO-тиждень для digest: 2026-W28
    version     INTEGER NOT NULL DEFAULT 1,  -- ++ при регенерації, старі лишаються
    html        TEXT NOT NULL,               -- самодостатня сторінка: стилі inline,
                -- дані inline JSON, нуль зовнішніх запитів
    frozen      INTEGER NOT NULL DEFAULT 0,  -- 1 = снепшот після session_end
    created_at  TEXT NOT NULL
);
CREATE INDEX idx_reports_ref ON reports(kind, ref_id, version);

CREATE TABLE task_checklist_items (
    id          INTEGER PRIMARY KEY,
    task_id     INTEGER NOT NULL REFERENCES tasks(id),
    seq         INTEGER NOT NULL,            -- порядок у Validation-секції промпта
    text        TEXT NOT NULL,
    status      TEXT NOT NULL DEFAULT 'pending',  -- pending | passed | failed
    checked_by  TEXT,                        -- heuristic | reporter | human
    event_id    INTEGER REFERENCES events(id),    -- доказ: клік веде в таймлайн
    checked_at  TEXT,
    UNIQUE(task_id, seq)
);

-- ============ Агенти і скіли (шар 3: менеджмент) ============

CREATE TABLE agents (
    id          INTEGER PRIMARY KEY,
    name        TEXT NOT NULL,               -- code-reviewer, db-migrator...
    scope       TEXT NOT NULL,               -- global | project
    project_id  INTEGER REFERENCES projects(id),  -- NULL для global
    file_path   TEXT NOT NULL,               -- .claude/agents/xxx.md
    model       TEXT,
    tools_json  TEXT,                        -- дозволені tools
    description TEXT,
    current_version_id INTEGER,              -- FK на agent_versions (deferred)
    deleted     INTEGER NOT NULL DEFAULT 0,
    UNIQUE(name, scope, project_id)
);

CREATE TABLE agent_versions (
    id           INTEGER PRIMARY KEY,
    agent_id     INTEGER NOT NULL REFERENCES agents(id),
    content_hash TEXT NOT NULL,              -- sha256 вмісту .md
    git_sha      TEXT,                       -- якщо конфіги в git
    content      TEXT NOT NULL,              -- повний вміст на момент версії
    created_at   TEXT NOT NULL,
    change_note  TEXT,                       -- заповнюється з UI-редактора
    UNIQUE(agent_id, content_hash)
);

CREATE TABLE skills (
    id          INTEGER PRIMARY KEY,
    name        TEXT NOT NULL,
    scope       TEXT NOT NULL,
    project_id  INTEGER REFERENCES projects(id),
    dir_path    TEXT NOT NULL,               -- папка зі SKILL.md
    description TEXT,                        -- з frontmatter — для лінтера
    current_version_id INTEGER,
    deleted     INTEGER NOT NULL DEFAULT 0,
    UNIQUE(name, scope, project_id)
);

CREATE TABLE skill_versions (
    id           INTEGER PRIMARY KEY,
    skill_id     INTEGER NOT NULL REFERENCES skills(id),
    content_hash TEXT NOT NULL,
    git_sha      TEXT,
    content      TEXT NOT NULL,              -- SKILL.md (ресурси — по ref)
    created_at   TEXT NOT NULL,
    change_note  TEXT,
    UNIQUE(skill_id, content_hash)
);

-- Результати лінтера конфігів (роздутий CLAUDE.md, агент без Boundaries...)
CREATE TABLE config_lint_findings (
    id         INTEGER PRIMARY KEY,
    target     TEXT NOT NULL,                -- agent:12 | skill:3 | claude_md:...
    rule       TEXT NOT NULL,                -- oversized_context | no_boundaries
    severity   TEXT NOT NULL,                -- info | warn | error
    message    TEXT NOT NULL,
    detected_at TEXT NOT NULL,
    resolved_at TEXT
);

-- ============ Метрики і evals (шар 4) ============

CREATE TABLE daily_rollups (
    day         TEXT NOT NULL,               -- YYYY-MM-DD
    project_id  INTEGER REFERENCES projects(id),
    agent_id    INTEGER REFERENCES agents(id),   -- NULL = увесь проєкт
    sessions    INTEGER NOT NULL DEFAULT 0,
    tasks_done  INTEGER NOT NULL DEFAULT 0,
    tasks_reverted INTEGER NOT NULL DEFAULT 0,
    tool_calls  INTEGER NOT NULL DEFAULT 0,
    errors      INTEGER NOT NULL DEFAULT 0,
    tokens_in   INTEGER NOT NULL DEFAULT 0,
    tokens_out  INTEGER NOT NULL DEFAULT 0,
    cost_usd    REAL    NOT NULL DEFAULT 0,
    wait_minutes REAL   NOT NULL DEFAULT 0,  -- час у waiting_approval
    PRIMARY KEY (day, project_id, agent_id)
);

CREATE TABLE eval_suites (
    id         INTEGER PRIMARY KEY,
    agent_id   INTEGER NOT NULL REFERENCES agents(id),
    name       TEXT NOT NULL,
    created_at TEXT NOT NULL
);

CREATE TABLE eval_cases (
    id        INTEGER PRIMARY KEY,
    suite_id  INTEGER NOT NULL REFERENCES eval_suites(id),
    prompt    TEXT NOT NULL,                 -- еталонна задача
    check_cmd TEXT,                          -- напр. go test ./... або скрипт
    expected  TEXT                           -- опис очікуваного результату
);

CREATE TABLE eval_runs (
    id               INTEGER PRIMARY KEY,
    suite_id         INTEGER NOT NULL REFERENCES eval_suites(id),
    agent_version_id INTEGER NOT NULL REFERENCES agent_versions(id),
    started_at       TEXT NOT NULL,
    finished_at      TEXT,
    passed           INTEGER,
    failed           INTEGER,
    tokens_total     INTEGER,
    cost_usd         REAL
);

CREATE TABLE eval_results (
    id        INTEGER PRIMARY KEY,
    run_id    INTEGER NOT NULL REFERENCES eval_runs(id),
    case_id   INTEGER NOT NULL REFERENCES eval_cases(id),
    status    TEXT NOT NULL,                 -- pass | fail | error
    session_id INTEGER REFERENCES sessions(id),
    notes     TEXT
);
```

### Ключові рішення і trade-offs

| Рішення | Чому | Ціна |
|---|---|---|
| SQLite, не Postgres | Локальний демон, один користувач, zero-ops; WAL тримає write-нагрузку хуків | Немає віддаленого доступу до БД — але API все одно через демон |
| `payload` JSON у events | Формат JSONL Claude Code — internal, змінюється | Частина запитів через `json_extract`; критичні поля винесені в колонки |
| Повний `content` у versions | Rollback і diff без залежності від git-стану | Розмір БД росте; агентські .md малі — прийнятно |
| `parent_event_id` | Дерево делегування (оркестратор → субагенти) одним полем | Рекурсивні CTE для глибоких дерев — SQLite вміє |
| `result_note` + `reverted` у tasks | Метрики якості потребують людського сигналу — мінімальний UX: один тап "ok / правив / відкат" після задачі | Дисципліна заповнення; без цього шар 4 не працює |

---

## 3. UI структура

### Навігація (sidebar / bottom bar на мобільному)

```
● Overview        — пульс системи
● Approvals  (3)  — черга дозволів, бейдж
● Sessions        — живі і минулі сесії
● Workspaces      — задачі/плани/артефакти по проєктах (фаза 3.5)
● Tasks           — черга задач
● Reports         — звіти: session / task / weekly digest / incident (фаза 2.5)
● Agents & Skills — реєстр + редактор
● Analytics       — cost / quality
● Health          — стан самого Swarmery
```

Пункт Reports з'являється у фазі 2.5 (див. §3.8); до того звіти доступні лише
кнопкою "Generate report" на деталі сесії.

### 3.1 Overview
- Активні сесії зараз (проєкт, агент, поточна дія, тривалість) — live через WS
- Pending approvals: топ-3 + кнопка в чергу
- Сьогодні: токени, $, задач виконано, помилок
- Спарклайн активності за 7 днів
- Останні завершені задачі зі статусом (ok / needs_review / failed)

### 3.2 Approvals
- Список pending: tool, що саме хоче зробити (згорнутий request_json), сесія, скільки висить
- Дії: Approve / Deny / Open session. Swipe-дії на мобільному
- Історія рішень (аудит: хто/звідки/коли)

### 3.3 Sessions
- Фільтри: проєкт, статус, агент, дата
- **Session detail** — головний екран, таби:
  - **Timeline**: промпт → tool calls → субагенти (розгортаються як піддерево) → skills → результат. Помилки червоним, out-of-scope зміни з прапорцем
  - **Diffs**: усі file_changes сесії, згруповані по файлах, unified diff
  - **Context**: які CLAUDE.md/rules підвантажені, розподіл токенів (історія vs системне vs tools), розмір контексту в часі
  - **Tree**: flame-graph делегування (по parent_event_id), час і токени на вузол
- Дії на живій сесії: Inject instruction, Kill

### 3.4 Tasks
- Kanban або список: queued → running → needs_review → done
- Створення задачі: проєкт, цільовий агент (опц.), spec-driven шаблон промпта (Context/Objective/Boundaries/Validation — преінжектиться)
- Ліміт паралельності per-проєкт, пріоритети drag-n-drop
- Після done — швидка оцінка: ✅ ok / ✏️ правив / ↩️ відкат (пише result_note/reverted)

### 3.5 Agents & Skills
- Реєстр: назва, scope (global/project), модель, останнє використання, задач за 30 днів, success-rate. Мертві агенти (0 використань 30+ днів) — приглушені
- **Agent detail**:
  - Метрики: задачі, % без правок, % відкатів, сер. токени/задача — по версіях
  - Версії: список agent_versions, diff між будь-якими двома, Rollback
  - Editor: форма (роль, модель, tools, boundaries) + raw .md таб з preview; Save = запис файлу + нова версія + git commit
  - Evals: suite агента, кнопка Run, історія прогонів по версіях
- **Sync view**: матриця агент × проєкт, дрейф версій, кнопка Push to all
- **Lint**: активні findings з config_lint_findings, по severity

### 3.6 Analytics
- Cost: по днях / проєктах / агентах / моделях; cost-per-task
- Quality: success-rate агентів у часі, топ error patterns, wait_minutes (втрачений час на approvals)
- Порівняння версій агента: до/після зміни промпта (A/B по метриках)

### 3.7 Health
- Стан колекторів: hooks (останній heartbeat), JSONL-watcher (lag), розмір БД
- Проєкти без хуків ("непідключені") + one-click інсталяція hook-конфігу
- Версія Claude Code на машині, попередження про зміну формату JSONL

### 3.8 Reports (фаза 2.5)

**Звіт = наратив + телеметрія.** Наратив генерує Reporter-агент: по завершенню
задачі (hook Stop або статус done) headless `claude -p` на дешевій моделі читає
транскрипт і повертає структурований JSON `{done[], decisions[{what,why}],
failures[], risks[], followups[]}` → таблиця `narratives`. Тільки для сесій
> N подій (default 30), з лімітом токенів; для решти — кнопка ручної генерації.
Reporter — теж агент системи, його робота видна у Swarmery.

- **Session/task report** — самодостатня HTML-сторінка (стилі inline, дані
  inline JSON, нуль зовнішніх запитів): наратив зверху → checklist (якщо є
  task) → diffs по файлах → тести (спроби → PASS) → субагенти → вартість/токени
  → out-of-scope. Зберігається в `reports` з version++ при регенерації.
  Експорт `.html` — shareable artifact без хмари.
- **Live view** `GET /live/{session_id}`: та сама сторінка + JS-підписка на
  `/api/ws` — checklist заповнюється, diffs доїжджають live; по session_end
  сторінка заморожується у снепшот (`frozen=1`).
- **Self-filling checklist**: при створенні задачі Validation-секція промпта
  парситься → `task_checklist_items`; евристики (Bash event з PASS + збіг
  ключових слів → passed, `checked_by=heuristic`, `event_id`=доказ); решту
  відмічає Reporter (`checked_by=reporter`); людина може перемкнути вручну.
  Кожна галочка клікабельна в таймлайн через `event_id`.
- **Weekly digest**: `swarmery digest [--week 2026-W28]` — другий виклик
  Reporter по наративах тижня, групування по проєктах (що зашипили, вартість,
  провали, топ follow-ups); `kind=weekly_digest` + endpoint і кнопка в UI.
- **Incident-звіт** для failed-задач: хронологія спроб (цикли edit→test→fail),
  місце зациклення, спалені токени до фейлу; генерується автоматично при
  `status=failed`.
- **API**: `GET /api/reports?kind=&ref=`, `GET /api/reports/{id}`,
  `POST /api/sessions/{id}/report`, експорт з `Content-Disposition: attachment`.
- **Reporter pipeline**: сесії з >30 подій; `claude -p --model <з конфігу>
  --output-format json`; retry 1 раз при битому JSON; не більше N авто-звітів
  на годину; guard від самотригерингу по cwd/маркеру.

#### Reporter: три неочевидні рішення

1. **Жорсткий поділ наратив/телеметрія.** Reporter повертає тільки текст
   (done/decisions/failures/risks/followups); УСІ числа у звіті (токени,
   вартість, тривалість, diff-статистика) беруться з БД. Числа з LLM
   ігноруються — галюцинована цифра гірша за відсутню.
2. **Guard від самотригерингу.** Сесії самого Reporter-а інджестяться як
   звичайні (система бачить сама себе), але ніколи не породжують
   report-of-report (детект по cwd/маркеру); плюс rate limit на авто-звіти
   і вартість Reporter-а видна в Analytics.
3. **Редакція секретів** перед тим, як транскрипт потрапляє в API-виклик
   Reporter-а — той самий фільтр, що й на ingest.

---

## 4. Порядок імплементації (маппінг на схему)

1. **MVP**: projects, sessions, turns, events, file_changes + екрани Overview/Sessions. Тільки JSONL-парсер.
   Backfill-scope: лише транскрипти формату Claude Code ≥2.1 (Agent tool, окремі sidechain-файли); pre-2.1 (`Task`, inline sidechains) — out of scope для MVP.
   Перед step 06 (ingest-демон): watch-експеримент — підтвердити, що транскрипти append-only, а не переписуються in place; дизайн tail-follow залежить від цього (Q11).
2. **Approvals**: permission_requests + hooks (PreToolUse повертає рішення з дашборда). Екран Approvals.

2.5. **Reporter + Reports**: narratives, reports, task_checklist_items; headless `claude -p` на Stop-hook; session/task-звіти, live view, weekly digest. (Agent D)

3. **Agents registry read-only**: agents, skills, *_versions (скан файлової системи + fsnotify). Реєстр без редагування.

3.5. **Workspaces** (розділ 7): workspace ingester (read-only), зшивка задача↔сесія, екран Workspaces. Наполовину заміняє п.5: workspace = system of record задач.

4. **Editor + git**: запис файлів, версіонування, rollback, lint.
5. **Tasks queue**: черга запуску headless-сесій ПОВЕРХ workspace-задач (створення задачі = `agent-work.sh init`).
6. **Rollups + Analytics**, потім **Evals**.
7. **Insights + MCP-памʼять** (розділ 5): loop detector → heatmap → MCP-сервер → mining.
8. **Developer superpowers** (розділ 6): по одному, за ROI.

Схема з п.1 вже містить усі FK для наступних шарів — міграції будуть адитивними (нові таблиці), без переписування ядра.

---

## 5. Фаза 4+: Insights і MCP-памʼять

Усе в цьому розділі будується поверх наявного ядра (`events`, `turns`,
`file_changes`, `narratives`) — тільки адитивні таблиці й нові read-шляхи,
ядро не переписується. Дані вже збираються з фази 1; тут вони починають
працювати на користувача.

### 5.1 Візуалізації

- **Codebase heatmap** — які файли/директорії агенти чіпають найчастіше
  (агрегат по `file_changes`): гарячі зони = кандидати на рефакторинг,
  на окремий skill або на жорсткіші boundaries в промптах.
- **Activity heatmap** — активність по днях тижня × годинах (по `events`),
  включно з часом, спаленим у `waiting_approval`: видно і власний розклад
  роботи з агентами, і де сесії простоюють, чекаючи людину.
- **Cost Sankey** — потік вартості: проєкт → сесія → агент/субагент → модель.
  Одна діаграма відповідає на "куди течуть гроші" краще за десять таблиць.
- **Agent quality curve** — success-rate / revert-rate агента в часі
  з анотаціями версій (`agent_versions`): видно, яка саме зміна промпта
  покращила чи зламала агента.

### 5.2 Pattern mining

- **Loop detector (real-time).** Демон дивиться на хвіст `events` живої
  сесії: N повторів циклу edit→test→fail по тих самих файлах = аномалія →
  рядок в `anomalies` + push-нотифікація. Це єдиний детектор, що працює
  live: є шанс зупинити сесію до того, як вона спалить бюджет.
- **Repeated-instruction mining.** `user_prompt`-и кластеризуються
  (embeddings або дешева модель): "ти написав це 14 разів — додати
  в CLAUDE.md чи зробити skill?" Результат — `instruction_clusters`
  з лічильником повторів і запропонованою дією.
- **Context-waste detector.** Сесії, де підвантажений контекст (роздутий
  CLAUDE.md, зайві rules) домінує над корисною роботою — кандидати
  на чистку; звʼязується з findings лінтера (`config_lint_findings`).

```sql
-- ============ Insights (фаза 4+): аномалії та кластери інструкцій ============

CREATE TABLE anomalies (
    id          INTEGER PRIMARY KEY,
    session_id  INTEGER NOT NULL REFERENCES sessions(id),
    kind        TEXT NOT NULL,               -- loop | context_waste | cost_spike
    detected_at TEXT NOT NULL,
    window_start_event_id INTEGER REFERENCES events(id),
    window_end_event_id   INTEGER REFERENCES events(id),
    score       REAL,                        -- впевненість детектора 0..1
    details     TEXT,                        -- JSON: файли циклу, лічильники повторів
    notified    INTEGER NOT NULL DEFAULT 0,  -- push уже надіслано
    resolved_at TEXT                         -- людина подивилась/закрила
);
CREATE INDEX idx_anomalies_session ON anomalies(session_id, detected_at);
CREATE INDEX idx_anomalies_open    ON anomalies(kind, resolved_at);

CREATE TABLE instruction_clusters (
    id           INTEGER PRIMARY KEY,
    project_id   INTEGER REFERENCES projects(id),  -- NULL = крос-проєктний патерн
    label        TEXT NOT NULL,              -- людський підпис кластера
    example_text TEXT NOT NULL,              -- репрезентативний промпт
    occurrences  INTEGER NOT NULL DEFAULT 0,
    first_seen   TEXT NOT NULL,
    last_seen    TEXT NOT NULL,
    suggestion   TEXT,                       -- claude_md | skill | ignore
    status       TEXT NOT NULL DEFAULT 'open'  -- open | actioned | dismissed
);
CREATE INDEX idx_ic_status ON instruction_clusters(status, occurrences DESC);
```

### 5.3 MCP-сервер: памʼять для агентів

`swarmery mcp` — stdio-процес з read-only доступом до SQLite. Claude Code
підключає його як звичайний MCP-сервер — і кожен агент отримує памʼять
про минулі сесії, рішення і фейли, якої не має жоден свіжий контекст.

Tools v1:

- `search_past_sessions` — повнотекстовий пошук (FTS5) по промптах і наративах
- `get_decisions` — минулі рішення з `narratives.summary_json` (decisions[])
  по темі/файлу
- `who_touched` — хто/коли/навіщо чіпав файл (`file_changes` + сесії)
- `get_failures` — минулі провали по темі: що ламалось і чим закінчилось
- `get_project_conventions` — вижимка конвенцій проєкту, накопичених у наративах

Правила:

- Відповіді компактні і token-limited: MCP-виклик не має роздувати контекст
  сесії, яка ним користується.
- Та сама редакція секретів, що й на ingest, — назовні не виходить нічого,
  чого немає права бачити транскрипту.
- Кожен MCP-виклик пишеться в `events` (телеметрія): в Analytics зʼявляється
  зріз "сесії з памʼяттю vs без" — чи справді памʼять покращує метрики,
  а не просто гріє контекст.

### 5.4 Predictions

kNN по embeddings завершених задач — без ML-інфраструктури:

- **Оцінка нової задачі**: ~час, ~вартість, ризик потрапити
  в needs_review/failed — по найближчих сусідах серед минулих задач.
- **Budget alerts**: прогноз перевитрати бюджету за поточним темпом.
- **Cost anomalies**: сесія коштує ×3 від подібних — сигнал подивитись,
  ще до того, як спрацює loop detector.

---

## 6. Developer superpowers (фаза 5+)

Кожен пункт — окрема, незалежно доставлювана фіча поверх наявних даних.
Впроваджуються по одному, за ROI (пріоритет — наприкінці розділу).

### 6.1 Conflict radar

Паралельні сесії, що чіпають ті самі файли: join `file_changes` по
`file_path` серед активних сесій. Шляхи нормалізуються worktree → parent
repo, щоб `/wt-feature/src/x.ts` і `/repo/src/x.ts` рахувались одним файлом.
Попередження в Overview до того, як виникне merge-конфлікт.

### 6.2 Session-scoped revert

З `file_changes` сесії будується revert-patch; на старті сесії —
auto-checkpoint (git tag або stash). Відмінність від `/rewind` у Claude Code:
працює **після** завершення сесії, крос-сесійно і з дашборда — не потрібен
живий REPL тієї самої сесії.

### 6.3 Model routing advisor

По історії: задачі цього класу дешевша модель закриває з тим самим
success-rate → підказка при створенні задачі "цю можна на дешевшій моделі".
Просто читає `tasks` + `turns` (модель, вартість, reverted) — жодного
нового збору даних.

### 6.4 Relay handoff

Передача задачі між сесіями/агентами без перечитування повного транскрипта:
сесія, що зупиняється, лишає компактний пакет (контекст, що зроблено,
що лишилось, відкриті питання) → наступна стартує з нього.

```sql
-- ============ Superpowers (фаза 5+): handoff між сесіями ============

CREATE TABLE handoff_packets (
    id              INTEGER PRIMARY KEY,
    task_id         INTEGER REFERENCES tasks(id),
    from_session_id INTEGER NOT NULL REFERENCES sessions(id),
    to_session_id   INTEGER REFERENCES sessions(id), -- NULL, поки не підхоплено
    packet_json     TEXT NOT NULL,           -- {context, done[], remaining[],
                    -- open_questions[]} — генерує Reporter-пайплайн
    created_at      TEXT NOT NULL,
    consumed_at     TEXT
);
CREATE INDEX idx_handoff_open ON handoff_packets(task_id, consumed_at);
```

### 6.5 Quality gates

`gates.yaml` у проєкті: перелік перевірок (tests pass, lint clean,
відсутність out-of-scope змін, diff ≤ N рядків). Демон проганяє gates
по done-задачі; результати пишуться в `task_checklist_items`
з `checked_by='gate'`; failed gate → задача повертається в `needs_review`.
Людська оцінка (`result_note`) лишається фінальним словом.

### 6.6 Playbooks

Багатокрокові сценарії (release, dependency bump, security patch):
послідовність spec-driven задач із залежностями; кожен крок — звичайний
рядок у `tasks`, тож увесь наявний конвеєр (черга, checklist, звіти,
gates) працює без змін.

```sql
-- ============ Superpowers (фаза 5+): playbooks ============

CREATE TABLE playbooks (
    id          INTEGER PRIMARY KEY,
    project_id  INTEGER REFERENCES projects(id), -- NULL = глобальний
    name        TEXT NOT NULL,
    definition  TEXT NOT NULL,             -- YAML/JSON: кроки, залежності,
                -- шаблони промптів (Context/Objective/Boundaries/Validation)
    created_at  TEXT NOT NULL,
    UNIQUE(project_id, name)
);

-- Адитивна міграція: крок playbook-а знає свій playbook
ALTER TABLE tasks ADD COLUMN playbook_id INTEGER REFERENCES playbooks(id);
```

### 6.7 Git/PR linkage

Звʼязка сесія → commit → PR: commit-події вже є в `events`; додатково
інджестяться pr-link записи (у MVP свідомо ignored). У Session detail
і звітах зʼявляються лінки на PR; статус PR-review повертається в task
як зовнішній сигнал якості — поруч із gates і людською оцінкою.

---

Пріоритет впровадження (за ROI, по одному): loop detector → conflict radar →
session revert → MCP memory → quality gates → playbooks → model routing →
relay handoff → mining → git/PR linkage.

---

## 7. Фаза 3.5: Workspaces — міст «намір ↔ телеметрія»

Swarmery бачить телеметрію (сесії, події, вартість), але не бачить наміру:
задачі, плани, фази й артефакти живуть у приватному workspace-репо
(`$AGENT_WORKSPACE_ROOT`, ним керує `agent-work.sh` з plugins/core). Фаза 3.5
зшиває ці два світи read-only інжестом. Розділ розміщено ПІСЛЯ §6, але в
порядку імплементації (§4) це п.3.5 — між Agents registry і Editor + git.

### 7.1 Принципи

1. **Workspace = system of record задач.** Задачі народжуються і живуть
   у workspace-репо (`agent-work.sh init|phase|complete`). Swarmery **НІКОЛИ
   не пише у workspace** — власник стану `agent-work.sh`; Swarmery лише
   читає, індексує і зшиває з телеметрією. Будь-яке «редагування задачі»
   з UI — це зміна рядків у власній SQLite, не файлів у workspace.
2. **Вміст файлів лишається на диску.** БД тримає шляхи, метадані та
   content hash (`task_artifacts`); markdown віддається on-demand через API
   із захистом від path-traversal (шлях береться ТІЛЬКИ з `task_artifacts`,
   ніколи з запиту) і з тією самою редакцією секретів, що на ingest.
3. **Project exclude-list діє і тут.** Проєкти, виключені з JSONL-колектора,
   не інжестяться і workspace-сканером — один спільний exclude-helper
   для обох колекторів.

### 7.2 Схема (адитивна міграція)

```sql
-- ============ Workspaces (фаза 3.5): міст «намір ↔ телеметрія» ============

CREATE TABLE workspaces (
    id           INTEGER PRIMARY KEY,
    slug         TEXT NOT NULL UNIQUE,       -- ім'я проєкту у workspace-репо
    root_path    TEXT NOT NULL,              -- $AGENT_WORKSPACE_ROOT/<slug>
    code_path    TEXT,                       -- overlay/project.json → codePath
    project_id   INTEGER REFERENCES projects(id),  -- зшивка з реєстром проєктів
                 -- (codePath ↔ projects.path, з нормалізацією symlink/слешів)
    display_name TEXT,
    last_scanned TEXT
);

-- workspace-задачі живуть у тій самій таблиці tasks (додаткові колонки):
ALTER TABLE tasks ADD COLUMN source TEXT NOT NULL DEFAULT 'queue';
                 -- 'queue' (створена з дашборда) | 'workspace' (інжест з диска)
ALTER TABLE tasks ADD COLUMN external_id TEXT;   -- yyyy-mm-dd-slug (task id картки)
ALTER TABLE tasks ADD COLUMN workspace_id INTEGER REFERENCES workspaces(id);
ALTER TABLE tasks ADD COLUMN archived_at TEXT;   -- потрапила в archive/
CREATE UNIQUE INDEX idx_tasks_workspace_external
    ON tasks(workspace_id, external_id)
    WHERE workspace_id IS NOT NULL;              -- UNIQUE(workspace_id, external_id)

CREATE TABLE task_phases (
    id           INTEGER PRIMARY KEY,
    task_id      INTEGER NOT NULL REFERENCES tasks(id),
    seq          INTEGER NOT NULL,
    name         TEXT NOT NULL,
    status       TEXT NOT NULL DEFAULT 'pending',  -- pending | active | done
    started_at   TEXT,
    completed_at TEXT,
    UNIQUE(task_id, seq)
);

CREATE TABLE task_artifacts (
    id           INTEGER PRIMARY KEY,
    task_id      INTEGER NOT NULL REFERENCES tasks(id),
    kind         TEXT NOT NULL,               -- readme | summary | plan | report | log | trace
    rel_path     TEXT NOT NULL,               -- відносно кореня task-директорії
    title        TEXT,
    mtime        TEXT,
    content_hash TEXT,                        -- вміст лишається на диску (§7.1 п.2)
    UNIQUE(task_id, rel_path)
);

CREATE TABLE task_sessions (
    task_id     INTEGER NOT NULL REFERENCES tasks(id),
    session_id  INTEGER NOT NULL REFERENCES sessions(id),
    link_source TEXT NOT NULL,                -- explicit | heuristic
    confidence  REAL,                         -- 0..1 для heuristic-звʼязків
    PRIMARY KEY (task_id, session_id)
);
```

### 7.3 Workspace ingester (read-only)

- **Сканер + fsnotify + періодичний rescan** по
  `$AGENT_WORKSPACE_ROOT/*/workspace/{working,archive}`; поява картки
  в `archive/` виставляє `tasks.archived_at`.
- **Директорія задачі**: `YYYY/MM/DD/<slug>` → `external_id = yyyy-mm-dd-slug`.
- **README-картка** парситься за конвенцією `- **Field**: value`;
  парсинг **толерантний**: відсутнє поле → NULL, зламана/нестандартна
  картка → задача все одно індексується (title з заголовка або slug),
  сканер ніколи не падає через один битий workspace.
- **Мапінг на проєкт**: `overlay/project.json → codePath` зіставляється
  з `projects.path` (нормалізація symlink-ів і trailing-slash) →
  `workspaces.project_id`.

### 7.4 Зшивка задача ↔ сесія

- **Explicit**: SessionStart-hook дописує `session_uuid` у
  `logs/sessions.md` активної картки — інжестер читає таблицю і створює
  `task_sessions(link_source='explicit')`. Хук живе у **plugins/core**
  (єдиний виняток із правила «Swarmery не пише у workspace» — пише не
  Swarmery, а плагін, який і є власником стану); зміна однорядкова +
  semver bump плагіна.
- **Heuristic**: `sessions.cwd ∈ workspaces.code_path` ∧ перекриття
  часового вікна сесії з активним вікном задачі → `link_source='heuristic'`
  з `confidence` 0..1. Евристичні звʼязки в UI малюються **пунктиром**;
  людина підтверджує або відхиляє (confirm → `link_source='explicit'`
  у БД, workspace не чіпається).

### 7.5 UI: екран Workspaces

- **Список workspace-ів**: лічильники working/archive + витрати за 7 днів ($).
- **Задачі проєкту**: список карток із phase stepper (`task_phases`);
  вартість задачі = Σ вартості звʼязаних сесій (`task_sessions`).
- **Картка задачі**: рендер README + SUMMARY, фази, deep-links на сесії
  (Session Detail), артефакти (markdown on-demand, §7.1 п.2), trace —
  завантажується on-demand.
- **Зворотний звʼязок**: chip задачі в Session Detail — з сесії видно,
  на яку задачу вона працювала.

### 7.6 Що це відкриває

- **Per-task метрики агентів і cost-per-task** — без ручного заповнення
  `tasks`: телеметрія агрегується по `task_sessions`.
- **Reporter (фаза 2.5) публікує наратив як артефакт задачі** через
  `agent-work.sh complete` — звіт стає частиною system of record.
- **MCP-памʼять (§5.3) шукає по планах і summary** з workspace, а не лише
  по транскриптах — агенти отримують памʼять про наміри, не тільки про дії.
