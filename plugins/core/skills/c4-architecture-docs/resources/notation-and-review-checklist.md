# C4 Notation Rules & Diagram Review Checklist

Distilled from the c4model.com mirror (`diagrams_notation.txt`, `faq.txt`,
`diagrams_checklist.txt`; captured 2026-07-08). Abstraction/level guidance is in
`c4-levels.md`; Mermaid-specific rendering in `mermaid-c4-syntax.md`.

**C4 is notation-independent** — it prescribes no colours or shapes. But the notation must
make sense and be comprehensible. **Litmus test: can each diagram stand alone and be (mostly)
understood without a spoken narrative?** If not, fix it against the checklist below.

---

## 1. Notation rules

### Diagrams

- **Title** — every diagram needs one describing **type + scope**
  (e.g. "System Context diagram for My Software System").
- **Key/legend** — every diagram needs one explaining the notation: shapes, colours, border
  styles, line types, arrow heads. This applies **even to UML/ArchiMate/SysML** — not everyone
  knows the notation.
- **Acronyms/abbreviations** — understandable by all audiences, or explained in the key.

### Elements

- **Type** — explicitly state each element's type (Person, Software System, Container,
  Component) so the abstraction level is unambiguous.
- **Description** — give a short description of at-a-glance responsibilities.
- **Technology** — every **Container and Component** must have a **technology explicitly
  specified** (e.g. "Next.js 16", "Python 3.12 / asyncio"). People and Software Systems don't
  need one.

### Relationships

- **Unidirectional** — every line is one direction (one arrow head).
- **Labelled** — label consistent with the arrow's direction and intent (dependency **or**
  data flow — see below). Be specific: **avoid single words like "Uses."** More explicit is
  better ("sends customer update events to" > "customer update events").
- **Protocol on inter-container edges** — any inter-process (inter-container) relationship
  must carry a **technology/protocol** label (e.g. "WebSocket", "HTTPS", "gRPC", "SQL").

### Dependency vs data flow (`faq.txt`)

Your choice — pick whichever tells the story better. Just make the **description match the
arrow direction**. Don't mix the two conventions without saying so in the key.

### Colours & shapes

- Blue/grey boxes are **not** dictated by C4 — use any colours.
- Keep colour coding **consistent** within and across the diagram set.
- Watch for **B&W printing** and **colour blindness** — don't rely on colour alone.
- If you use icons (incl. cloud provider icons on deployment diagrams), **put them in the key.**

### Scaling — don't cram (`faq.txt`)

Don't force the whole story onto one diagram — you run out of canvas or drown in overlapping
lines (too-high cognitive load; "if nobody understands it, nobody looks at it"). **Split one
complex diagram into several simpler ones**, each focused on a business/functional area,
bounded context, use case, or feature set — **each at the same level of abstraction.** For
microservice-heavy container views, prefer N per-service diagrams (each showing nearest
inbound/outbound dependencies) over one dense all-in-one.

---

## 2. Software architecture diagram review checklist

Reproduced faithfully from `diagrams_checklist.txt` — each item is a Yes/No question. Run it
against **every** diagram before sharing. A "No" is a defect to fix, not a note to explain away.

### General

- [ ] Does the diagram have a title?
- [ ] Do you understand what the diagram **type** is?
- [ ] Do you understand what the diagram **scope** is?
- [ ] Does the diagram have a **key/legend**?

### Elements

- [ ] Does every element have a **name**?
- [ ] Do you understand the **type** of every element? (level of abstraction — software system, container, etc.)
- [ ] Do you understand **what every element does**?
- [ ] Where applicable, do you understand the **technology choices** associated with every element?
- [ ] Do you understand the meaning of all **acronyms and abbreviations** used?
- [ ] Do you understand the meaning of all **colours** used?
- [ ] Do you understand the meaning of all **shapes** used?
- [ ] Do you understand the meaning of all **icons** used?
- [ ] Do you understand the meaning of all **border styles** used? (e.g. solid, dashed)
- [ ] Do you understand the meaning of all **element sizes** used? (e.g. small vs large boxes)

### Relationships

- [ ] Does every arrow have a **label** describing the intent of that relationship?
- [ ] Does the **description match the relationship direction**?
- [ ] Where applicable, do you understand the **technology choices** associated with every relationship? (e.g. protocols for inter-process communication)
- [ ] Do you understand the meaning of all **acronyms and abbreviations** used?
- [ ] Do you understand the meaning of all **colours** used?
- [ ] Do you understand the meaning of all **arrow heads** used?
- [ ] Do you understand the meaning of all **line styles** used? (e.g. solid, dashed)

---

## 3. Modelling over diagramming (advice) — `diagrams_notation.txt`, `tooling.txt`

A *diagramming* tool speaks only "boxes and lines" — it can't validate or query, and reuse is
copy-paste (rename a box → rename everywhere). A *modelling* tool builds one non-visual model
(all elements + relationships defined once) and renders multiple **views** on top. "A model is
just data" (a directed graph of nodes/edges) — enabling querying, alternative visualisations,
export, and AI. The site explicitly favours **modelling**; recommended progression is to model
first, then generate diagrams as views. For a diagrams-as-code workflow, the `.mmd`
sources are the closest practical analogue — keep the C4 vocabulary explicit in source so
intent survives.
