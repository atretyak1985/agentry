# C4 Abstractions & Diagram Levels

Distilled from the c4model.com mirror (Simon Brown; captured 2026-07-08). This is the
"what each box means and when to draw it" reference. Notation rules live in
`notation-and-review-checklist.md`; Mermaid syntax in `mermaid-c4-syntax.md`.

Core idea: C4 makes **"maps of your code"** at zoom levels — like Google Maps.
Model the **abstractions** once; every diagram is just a **view** (zoom level) over that
model. Works for up-front design *and* retrospective documentation of existing code.

---

## 1. The abstraction hierarchy

Source: `abstractions.txt` and its per-abstraction pages.

> A **Software System** is made of one or more **Containers** (apps + data stores), each
> holding one or more **Components**, each implemented by one or more **Code** elements.
> **People** use the software systems we build.

| Abstraction | What it is | Rule of thumb / gotcha |
|---|---|---|
| **Person** | Actor, role, persona, or named individual who uses the system | The humans (and non-human users) around the edges |
| **Software System** | *Highest* abstraction; delivers value. Includes your system **and** the systems it depends on / that depend on it | Scope = **what one team builds, owns, and can see inside of** — often one repo; boundary ≈ team boundary; usually deploys together. **NOT** a product domain, bounded context, or squad |
| **Container** | An **application or a data store** — *something that must be running* for the system to work; a **runtime boundary** | **"Not Docker!"** Deployment is a *separate* concern (see §5). Examples below |
| **Component** | A **grouping of related functionality behind a well-defined interface** | **NOT separately deployable** — the *container* is the deployable unit; all components share one process space |
| **Code** | Classes, interfaces, enums, functions, objects — the language's building blocks | Components are made of these |

### Container — "Not Docker!" (`abstractions_container.txt`)

A container names nothing about physical execution. Valid containers:
server-side web app, client-side SPA, desktop app, mobile app, console app,
**serverless function**, database/schema, blob/content store or CDN, file system, shell script.

- A **JAR / assembly / DLL / module is NOT a container** — that is compile-time organisation,
  not a runtime unit.
- **Data services you don't run yourself** (S3, RDS, Azure SQL, a CDN) **ARE** containers —
  you own the buckets/schemas; they're integral to your architecture, just hosted elsewhere.
- **Web app — one container or two?** Mostly-static-HTML server app = **one**. Ships
  significant client JS (e.g. an Angular/React SPA) = **two** separate containers (two process
  spaces talking over JSON/HTTPS).

### Component (`abstractions_component.txt`)

Mental model (OO): implementation classes behind an interface. Per language: OO →
classes+interfaces; C → C files in a directory; JS → a module of objects/functions;
functional → a module grouping functions/types. A JAR/package/namespace/folder is
typically **not** a component (C4 shows runtime partitioning, not org units). Small
codebases: identify components manually; large: automate via reverse-engineering.

---

## 2. Microservices → abstractions (`abstractions_microservices.txt`)

Decided by **ownership**:

- Services are an **implementation detail inside one system owned by one team** → model
  **each microservice as a group of one or more containers** (e.g. an API container +
  a DB-schema container). The container diagram does *not* draw the microservice as an
  explicit box — use colour coding or a box around each API/DB pair. The message bus is
  not the container.
- Conway's Law splits ownership so **each service is owned by a separate team** → "promote"
  each service to its **own software system**, diagrammed from that team's perspective.

## 3. Queues & topics → abstractions (`abstractions_queues-and-topics.txt`)

- **Incorrect:** the message bus as a single C4 container ("hub and spoke" hides real
  producer/consumer coupling).
- **Correct (option A):** model **each queue/topic as a C4 container** — a queue *is* a data
  store. Exposes point-to-point coupling (A → queue X → C).
- **Correct (option B):** model it **implicitly** with a **"via"** notation — omit the queue
  box, put the queue name on the arrow (simpler, queue less explicit). Use dashed-vs-solid or
  colour to distinguish messaging from API calls. Pub/sub: flip arrow directions to highlight
  publisher/subscriber roles.
- With multiple systems, also decide who **owns** each queue/topic (owner defines message
  format + operation).

## 4. Changing terminology / adding levels (`abstractions_faq.txt`, `faq.txt`)

You **may change terminology** (e.g. "module"/"function" for functional languages) — as long
as everyone explicitly understands it. Adding *more levels* is an **advanced manoeuvre**: most
"C4 is too limiting" complaints come from misunderstanding it or trying to model
*organisational groupings* (subsystems, bounded contexts, layers, libraries) as abstractions.
The real fix is precise terminology. Only add levels for a genuine, rigorously-defined need.

---

## 5. The seven diagram types

The model is *named after* the four static-structure diagrams. **You don't need all four —
only those that add value; System Context + Container suffice for most teams.** Then three
supplementary types.

### Core (static structure) — `diagrams_*.txt`

| # | Level | Scope | Primary elements | Audience | Recommended? |
|---|---|---|---|---|---|
| 1 | **System Context** | one software system | the system in scope + its users + neighbouring systems | **Everybody** (technical + non-technical) | **Yes — all teams** |
| 2 | **Container** | one software system | containers within it | Technical: architects, devs, ops/support | **Yes — all teams** |
| 3 | **Component** | one container | components within that container | Architects & developers | **No** — only if it adds value; consider automating |
| 4 | **Code** | one component | classes, interfaces, functions, DB tables… | Architects & developers | **No** — IDE generates on demand |

- **System Context** — the recommended *starting point*: step back, see the big picture.
  System as a box in the centre, surrounded by users and interacting systems. **Detail is
  not important** — focus on **people and software systems, not technologies/protocols.**
  Showable to non-technical people.
- **Container** — zoom inside the system boundary: the **high-level shape**, how
  responsibilities are distributed, the **major technology choices**, and how containers
  communicate. Says little about deployment — that varies per environment (→ deployment
  diagrams, one per environment).
- **Component** — zoom into one container: its components, responsibilities, technology.
  **Optional.** For long-lived docs, **consider automating** creation (reverse-engineering)
  to keep it true.
- **Code** — zoom into one component (UML class diagram, ERD, etc). **Very much optional**;
  usually available on-demand from the IDE. Show only the attributes/methods needed to tell
  the story. **Not recommended** for anything but the most important/complex components, and
  discouraged for long-lived docs — IDEs regenerate it.

**When NOT to draw:** skip Component and Code unless they add concrete value. Code diagrams
especially — prefer (1) don't create, or (2) generate on demand from the IDE — they go stale
fastest under active development.

### Supplementary — `diagrams_system-landscape/dynamic/deployment.txt`

| Type | Scope | Purpose | Audience | Recommended? |
|---|---|---|---|---|
| **System Landscape** | enterprise / org / department | A **map of software systems** in scope — a context diagram *without* focus on one system | Technical + non-technical | **Yes**, esp. larger orgs ("a bridge into enterprise architecture") |
| **Dynamic** | a feature / story / use case | How static-model elements **collaborate at runtime**; **numbered interactions** for ordering | Technical + non-technical (inside & outside the team) | **No** — use sparingly |
| **Deployment** | one environment (prod/staging/dev) | How instances **map onto infrastructure** | Technical (+ infra architects, ops) | **Yes** |

- **Dynamic** — based on a UML *communication* diagram: free-form layout with numbered
  interactions; can also be drawn UML *sequence*-style (same info, different layout). Elements
  are your choice — systems, containers, or components. Use for interesting/recurring patterns
  or a feature needing a complicated set of interactions. **When NOT to draw:** simple,
  self-evident flows — reserve it for genuinely complex collaborations.
- **Deployment** — based on a UML deployment diagram. A **deployment node** = where an
  instance runs (physical; VM/IaaS/PaaS; container/Docker; or an execution environment like a
  DB server); **nodes nest**. May include **infrastructure nodes** (DNS, load balancers,
  firewalls). Cloud icons (AWS/Azure) are allowed **but must appear in the diagram key**.
  Draw **one per environment** — the concern deployment diagrams exist to capture.

---

## 6. How each level ages (keeping diagrams current) — `diagrams_faq.txt`

| Level | Ages | Keep current by |
|---|---|---|
| System Context | very slowly | hand-maintain; service catalogs (Backstage, ServiceNow) |
| Container | relatively slowly (faster under heavy microservices/serverless) | log parsing / OpenTelemetry |
| Component | frequently under active development | static analysis / reverse-engineering |
| Code | very quickly | **don't create, or generate on demand** |

Hand-drawn diagrams go stale; **auto-generation keeps them reflecting reality.** Deployment
diagrams can be reverse-engineered from IaC (Terraform, CloudFormation).

## 7. Applicability limits — `faq.txt`

C4 is for **custom-built, bespoke** systems (monolith or distributed, any language, on-prem or
cloud). Less suited: **embedded systems/firmware** and heavy-customisation platforms (SAP,
Salesforce) — though even there, System Context + Container may still help. C4 deliberately
does **not** cover business processes, workflows, state machines, or domain/data models —
supplement with UML / BPML / ERDs. C4 does **not** imply a design process or org structure —
levels do **not** map to roles.
