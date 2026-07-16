---
name: c4-architecture-docs
version: "1.0.0"
owner: "swarmery-core"
description: "Use this skill to document the architecture of a big issue / epic / feature with the C4 model (c4model.com) -- producing system context, container, component, and dynamic diagrams as Mermaid .mmd plus a narrative doc. Trigger phrases: 'C4 model', 'architecture diagram', 'system context / container / component diagram', 'document the architecture of this epic', 'draw the containers for this feature'. Don't use it for: rendering an existing .mmd to HTML (use the project's Mermaid viewer/renderer, or the `mmdc` CLI); FigJam whiteboard diagrams (use figma:figma-generate-diagram); a pure decision record with no diagrams (use the ADR template — `.claude/templates/adr-template.md`, falling back to core's `templates/adr-template.md`)."
color: cyan
---

# Purpose

Produce house-consistent **C4 architecture documentation** for a big issue: a small set of Mermaid C4 diagrams (`.mmd`) plus a short narrative doc, filed in the task dir and rendered for review. C4 gives "maps of your code" at fixed zoom levels (System Context -> Container -> Component -> Code, plus supplementary types — System Landscape, Dynamic, Deployment) so every diagram is typed, labelled, and self-explanatory instead of "a confused mess of boxes and lines" (c4model.com, Simon Brown).

This skill owns the **HOW** — level selection, diagram grammar, boundary/technology-labelling rules, and the file-and-promote workflow. It authors `.mmd` source and narrative; it does **not** render (hand the `.mmd` to the project's Mermaid viewer/renderer) and it does **not** replace an ADR (that is the ADR template).

The project's own tiers, repos, and containers are **not** baked into this skill — read them from `.claude/project.json` (`repos`, `apps`, `mainApp`, `device`, `stack`) and the project's `CLAUDE.md`. Ground every box in that real inventory.

# When to use this skill

- User says "document the architecture", "draw the C4 / system context / container / component diagram", "give me an architecture diagram for this feature/epic".
- A big issue (epic, cross-tier feature, incident post-mortem, audit) needs a durable structural picture before or after implementation.
- `@architecture-designer` reaches its "design architecture" step and needs the C4 grammar.
- You need to show how the project's containers (from `.claude/project.json` → `repos`/`apps`/`stack`) fit together for a specific change.

# When NOT to use this skill

- **You already have a `.mmd` and just want to view it** -- render it with the project's Mermaid viewer skill (if it has one) or the `mmdc` CLI.
- **You want a FigJam / freeform whiteboard diagram** -- use `figma:figma-generate-diagram`.
- **You only need to record a decision** (options, trade-offs, consequences) with no structure diagram -- write an ADR from the ADR template.
- **You need business-process / workflow / state-machine / data-model detail** -- C4 deliberately omits these; supplement with the relevant diagram type, not a C4 view.
- **A tiny single-file change** -- an architecture doc is over-documenting; a one-line PR note suffices.

# Required environment

- `rg` / `Glob` / `Read` to inventory the affected repos before drawing (repo list from `.claude/project.json` → `repos`).
- A task dir under `${AGENT_WORKSPACE_ROOT}/${AGENT_PROJECT}/workspace/working/YYYY/MM/DD/{slug}/` (create via `agent-work.sh init` or `mkdir -p`).
- A Mermaid renderer to view `.mmd` for review — the project's viewer skill if it ships one, otherwise the `mmdc` CLI (`@mermaid-js/mermaid-cli`).

# Inputs

| Input | Required | Description |
|-------|----------|-------------|
| Issue / epic scope | Yes | The big issue to document (issue-tracker id, MR, or prose). Sets which containers are touched |
| Affected repos | Yes | Which of the project's repos (`.claude/project.json` → `repos`) the change spans |
| Task dir / slug | No | Existing `working/YYYY/MM/DD/{slug}/`; created if absent |
| Promotion target | No | The project's architecture docs directory by default (matches `@architecture-designer`'s `{component}-design.md` output path) if the doc is durable + cross-team; another project docs area only for a topic-specific doc (ASK-gated if it touches a contract) |

# Outputs

**Format:** Mermaid `.mmd` source files + one narrative markdown doc, in the task `reports/`.

**Length budget:** Narrative doc **<= 200 lines** (diagrams carry the detail; prose only frames them). One `.mmd` per chosen level. This SKILL.md stays under 250 lines. Keep the final chat summary within the project's compression conventions.

**Contents:**
- `reports/architecture.md` — narrative, structured per `templates/architecture-doc-template.md`.
- `reports/c4-l1-context.mmd`, `reports/c4-l2-container.mmd`, and `reports/c4-l3-<container>.mmd` / `reports/c4-dynamic-<flow>.mmd` as chosen.
- Rendered `.html` per `.mmd` (via the project's Mermaid renderer).
- Linked ADR(s) for each boundary/technology decision.

# Procedure

<procedure>

1. **Scope the issue.** Classify it (epic / feature / incident / audit) and state the big issue in 2-3 sentences: what is in and out of scope. Read `resources/c4-levels.md` for the abstraction model (Person / Software System / Container / Component / Code) before drawing.
   Checkpoint: one-paragraph scope written; you can name the systems the issue touches.

2. **Pick the minimum C4 levels.** Over-diagramming is the common failure — draw only levels that carry the argument (decision table below; full rationale in `resources/c4-levels.md`).

   | Level | Diagram | Include when |
   |---|---|---|
   | L1 System Context | who/what talks to the system | **Always** — one diagram sets the frame |
   | L2 Container | deployable units + their wires | **Always** — where most issues live |
   | L3 Component | internals of *one* container | Only for a container this issue actually changes |
   | L4 Code | class/function detail | Almost never — only a load-bearing algorithm; prefer IDE-generated |
   | Dynamic | numbered runtime collaboration | Only for a tricky multi-step flow (e.g. a command handshake) |
   | Deployment | per-environment infra map | Only when the issue changes how containers map onto infra; see `resources/c4-levels.md` §5 |

   Rule of thumb: **L1 + L2 for every big issue**; add one L3 per touched container; add a Dynamic view only for a non-obvious runtime sequence.
   Checkpoint: a level list with a one-line "why" each (seeds section 2 of the doc).

3. **Inventory the real boxes.** With `rg` / `Glob` / `Read`, confirm which containers the issue touches. Do **not** invent systems or protocols — ground every box in the project's real inventory (`.claude/project.json` → `repos`/`apps`/`stack`, the project `CLAUDE.md`, and the worked examples in `examples/`). Flag any of the project's safety-critical / do-not-touch paths now (the project's NEVER/ASK rules, if it defines them) — these change the review requirements.
   Checkpoint: box/relationship list matches the inventory; do-not-touch overlaps noted.

4. **Author the `.mmd` diagrams — after a complexity gate.** BEFORE writing any Mermaid C4, count the
   boxes and relationships from the step-3 inventory. **If a single view exceeds ~10 elements or
   ~12 relationships (or a Dynamic view exceeds ~10 steps), do NOT use Mermaid C4 for that view** —
   Mermaid's C4 auto-layout has no manual positioning and reliably produces crossing lines and
   overlapping labels at that density (an 18-edge L3 renders unreadable).
   Instead: split into several smaller views at the same abstraction level, or author a plain
   `flowchart` with C4 styling conventions (mapping table in `resources/mermaid-c4-syntax.md` §3).
   Under the gate: copy the matching starter skeleton and fill it: `templates/starter-system-context.mmd` (L1), `templates/starter-container.mmd` (L2), `templates/starter-component.mmd` (L3), `templates/starter-dynamic.mmd` (Dynamic). Follow the notation rules in `resources/notation-and-review-checklist.md` (every element typed + described; every container/component technology-labelled; every relationship unidirectional, labelled, and protocol-labelled for inter-container wires) and the Mermaid syntax + gotchas in `resources/mermaid-c4-syntax.md` (hex colors only — never OKLCH; quote labels containing `,` `(` `)`; no sprites/`!include`). Study the worked examples in `examples/` for the idiom.
   Checkpoint: element/relationship counts recorded per view and each is under the gate (or the
   fallback/split was taken); each `.mmd` passes the review checklist in `resources/notation-and-review-checklist.md` (title + key + typed elements + labelled directed relationships).

5. **Write the narrative doc.** Fill `templates/architecture-doc-template.md` into `reports/architecture.md` (metadata -> problem/scope -> levels used -> context -> container -> component -> cross-tier contracts -> decisions -> risks -> diagram index). In the cross-tier section, cite the project's contract / interface doc (if it maintains one) as the source of truth for any inter-container wire.
   Checkpoint: doc <= 200 lines; every chosen `.mmd` referenced from a section.

6. **Render for review — and LOOK at every diagram.** Render each `.mmd` with the project's Mermaid viewer/renderer; default output is same dir + basename + `.html` in `reports/`. Do **not** export PNG/PDF as deliverables (out of scope). Follow the `browser-verification` skill for the render+screenshot mechanics, then **take a screenshot of each rendered diagram and visually inspect it** — "renders without console errors" is NOT the readability bar; auto-layout sprawl produces perfectly error-free garbage. A diagram fails visual review if lines cross each other, labels overlap boxes/labels, or boxes touch. On failure, apply the step-4 fallback (split the view or rewrite as a styled `flowchart`) — do not ship it with a note.
   Checkpoint: each `.mmd` renders with zero console errors **AND its screenshot passed visual review (no crossing lines, no overlapping labels)**; `.html` paths recorded in the doc's diagram index.

7. **Record decisions as ADR(s).** For each boundary or technology choice the diagrams embody, write an ADR from the ADR template (`.claude/templates/adr-template.md`, falling back to core's `templates/adr-template.md`) into `reports/adr-NNN-<slug>.md` and link it from the doc's Decisions section.
   Checkpoint: every non-obvious structural decision has a linked ADR.

8. **File per workspace rules.** Ensure the task `README.md` card exists and write `SUMMARY.md` at completion. Never leave loose `*.md` at the workspace root — `protect-sensitive-files.sh` blocks it. Promote a settled copy to the project's architecture docs directory (the default, aligned with `@architecture-designer`'s `{component}-design.md` output path) **only** when durable + cross-team; use another project docs area only for a topic-specific doc, and if the promotion touches a contract, ASK first (the project's ASK policy).
   Checkpoint: task-dir artifacts complete; promotion (if any) is deliberate and, when contract-touching, approved.

</procedure>

## Quick reference

| Level | Mermaid decl | Starter | Draw when |
|---|---|---|---|
| System Context | `C4Context` | `templates/starter-system-context.mmd` | Always |
| Container | `C4Container` | `templates/starter-container.mmd` | Always |
| Component | `C4Component` | `templates/starter-component.mmd` | Per touched container |
| Dynamic | `C4Dynamic` | `templates/starter-dynamic.mmd` | Tricky runtime flow |
| Deployment | `C4Deployment` | — | Per-environment infra topology |
| Code (L4) | — (IDE/ERD) | — | Almost never |

Reference files: levels -> `resources/c4-levels.md`; notation + checklist -> `resources/notation-and-review-checklist.md`; Mermaid syntax + gotchas -> `resources/mermaid-c4-syntax.md`. Worked examples -> `examples/`.

# Self-check before returning

- [ ] Chosen levels are the *minimum* that carries the argument (L1+L2 always; L3 only per touched container; Dynamic only if the flow is non-obvious).
- [ ] Every box maps to a real system/container from the project's inventory (`.claude/project.json`, `CLAUDE.md`) — no invented boxes or protocols.
- [ ] Every element is typed + described; every container/component has a technology; every relationship is directed, labelled, and protocol-labelled for inter-container wires.
- [ ] Every view is under the step-4 complexity gate (~10 elements / ~12 relationships / ~10 dynamic steps) or was split / rewritten as a styled `flowchart`.
- [ ] Each `.mmd` passes the `resources/notation-and-review-checklist.md` review checklist, renders without console errors, **and its screenshot passed visual review** (no crossing lines, no overlapping labels).
- [ ] Narrative doc <= 200 lines and references every diagram; cross-tier section cites the project's contract doc where one exists.
- [ ] Do-not-touch overlaps flagged with required reviews; decisions captured in linked ADR(s).
- [ ] Artifacts inside the task dir; `SUMMARY.md` written; promotion (if any) is durable + cross-team (contract touch = ASK).

# Common mistakes to avoid

| Red flag / symptom | Fix |
|---|---|
| Drew all four C4 levels for a two-container change | Draw only what adds value — L1+L2 for most issues; skip Component/Code unless the issue changes that container's internals |
| Modeled a Docker container / message bus as a C4 Container | A C4 Container is a runtime app or data store, not Docker. Model queues/topics as data-store containers or a "via" relationship — never the bus as a hub (see `resources/c4-levels.md`) |
| Relationship labelled "Uses" with no protocol | Be specific and protocol-label inter-container wires ("Streams events [WebSocket]"), per `resources/notation-and-review-checklist.md` |
| Invented a service/protocol that isn't in the project | Ground every box in `.claude/project.json` + the project `CLAUDE.md` + `examples/`; if unsure, `rg` the repos |
| Used `oklch(...)` in `UpdateElementStyle` or unquoted a comma in a label | Hex/rgba colors only; quote any label with `,` `(` `)` — Mermaid C4 gotchas in `resources/mermaid-c4-syntax.md` |
| Wrote the doc as a loose file at the workspace root | Blocked by `protect-sensitive-files.sh`; write inside `working/YYYY/MM/DD/{slug}/reports/` |
| Verified "renders with zero console errors" and shipped without looking at the picture | Console-clean ≠ readable. Screenshot every diagram and inspect it (step 6); crossing lines / overlapping labels = apply the step-4 fallback |
| Diagram destined for a STATIC surface (artifact, PDF, README image) authored with Mermaid C4 | The readability bar is higher with no pan-zoom crutch: keep the density well under the step-4 gate, or hand-lay the SVG / use a styled `flowchart` for that surface; keep the `.mmd` as the diffable source |

# What to surface to the user

- The task-dir path and each `.mmd` -> rendered `.html` pair.
- Which C4 levels were drawn and why (the level table from step 2).
- Any project do-not-touch path the change touches and the review it triggers.
- Whether a promotion to the project's architecture docs directory is warranted — and, if it touches a contract, the ASK.

# Escalation

- **Scope spans >1 repo with a contract change** -- surface merge order and delegate coordination back to `@architecture-designer` / the orchestrator; a contract edit is an ASK (the project's ASK policy).
- **A diagram needs a level/type C4 doesn't provide** (state machine, ERD, business process) -- supplement with the right diagram type; don't force it into a C4 view.
- **Auto-layout sprawls or overlaps** -- fall back to a plain `flowchart` with C4 styling conventions (mapping table in `resources/mermaid-c4-syntax.md`).

# Examples

<example title="Container view for a cross-tier feature">

**Input:** Epic "persist per-order event history for replay" touching the API server + the relational DB.
**Process:** Scope -> pick L1+L2 (+ one L3 for the API server container) -> inventory boxes against `examples/example-container.mmd` -> fill starters -> write `architecture.md` -> render via the project's viewer -> ADR for the append-only-log-vs-snapshot-table choice.
**Output:** `reports/{architecture.md, c4-l1-context.mmd, c4-l2-container.mmd, c4-l3-api-server.mmd, adr-001-event-store.md}` + rendered `.html`.

</example>

<example title="Dynamic view of a request flow">

**Input:** Document the checkout/payment command path for an incident review.
**Process:** L1+L2 for frame + one `C4Dynamic` from `templates/starter-dynamic.mmd`, modeled on `examples/example-dynamic.mmd` (auto-numbered `Rel` steps browser -> API server -> payment gateway -> DB).
**Output:** `reports/c4-dynamic-checkout.mmd` + `.html`, linked from `architecture.md` section 6.

</example>

# Failure modes

| Failure | Recovery |
|---------|----------|
| Blank render in the viewer | Check console for `Unsupported color format: "oklch(...)"` — convert `UpdateElementStyle` colors to hex (`resources/mermaid-c4-syntax.md`) |
| Parse error on a label | A literal `,` `(` `)` `:` in an unquoted label — double-quote the label; avoid nested `"` |
| Diagram is unreadable / sprawls | Split into per-area diagrams at the same abstraction level, or fall back to `flowchart` with C4 styling |
| Reviewer can't tell element types apart | Add a key and ensure every element uses a typed macro (`Person`/`System`/`Container`/`Component`); rerun the checklist |
| Doc rejected as "over-documented" | Cut to L1+L2; move Component detail to an appendix or drop it |

# Related skills

- **browser-verification** — the render + screenshot + visual-inspection mechanics for step 6.
- **summary-templates** — format the completion `SUMMARY.md` / work summary (user-invoked).
- **html-reporting** — wrap a multi-section narrative architecture report in the canonical shell.
- **`@architecture-designer` agent** — primary caller; this skill supplies the C4 grammar for its design step.
- A project's own **Mermaid viewer skill** (if it ships one) renders the `.mmd` this skill authors into interactive HTML (step 6).

## File layout

```
c4-architecture-docs/
  SKILL.md                                    (this file)
  resources/
    c4-levels.md                              (C4 abstractions + 4 levels + supplements; when to use/stop)
    notation-and-review-checklist.md          (notation rules + diagram review checklist)
    mermaid-c4-syntax.md                      (Mermaid C4 syntax + gotchas + viewer integration)
  templates/
    architecture-doc-template.md              (big-issue narrative doc template)
    starter-system-context.mmd                (minimal C4Context skeleton)
    starter-container.mmd                      (minimal C4Container skeleton)
    starter-component.mmd                      (minimal C4Component skeleton)
    starter-dynamic.mmd                        (minimal C4Dynamic skeleton)
  examples/
    example-system-context.mmd                (neutral: Orders Platform system context)
    example-container.mmd                      (neutral: SPA + API server + DB containers)
    example-dynamic.mmd                        (neutral: checkout/payment command flow)
```
