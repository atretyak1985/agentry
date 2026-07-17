package api

// Analytics uplift: GET /api/stats/skills — per-skill invocation/error/denied
// counts and duration stats (avg + p95) over a local-day range, with a
// per-agent split for the expandable row in the Skills panel
// (web/src/pages/Analytics.tsx). Mirror of statsTools, but grouped by the skill
// NAME rather than the tool name.
//
// A skill invocation is a `skill_use` event; the skill name lives in
// json_extract(payload, '$.input.skill') — the same expression the runs
// breakdown uses (analytics.go runKind["skill"]), so counts agree across pages.
// Agent attribution is identical to statsTools: a sidechain event is parented
// (parent_event_id) to its subagent_start, whose payload carries subagent_type;
// a NULL parent is the orchestrator ("main").

import (
	"database/sql"
	"net/http"
	"sort"
)

type skillStatDTO struct {
	Skill  string         `json:"skill"`
	Calls  int64          `json:"calls"`
	Errors int64          `json:"errors"`
	Denied int64          `json:"denied"`
	AvgMs  *float64       `json:"avg_ms"` // nil when no invocation carried a duration
	P95Ms  *int64         `json:"p95_ms"` // nil when no invocation carried a duration
	Agents []toolAgentDTO `json:"agents"`
}

type skillsDTO struct {
	From   string         `json:"from"`
	To     string         `json:"to"`
	Skills []skillStatDTO `json:"skills"`
	// Agents lists every attributed agent in the range (NOT narrowed by the
	// ?agent= filter) — the option set for the panel's agent dropdown.
	Agents []string `json:"agents"`
	// Approx: true when the range overlaps pruned (rolled-up) days — rollups
	// carry no per-skill events, so the counts silently undercount there.
	Approx bool `json:"approx"`
}

// GET /api/stats/skills?from&to&project&agent — project is the optional global
// scope (slug or id). agent optionally narrows every row + column to a single
// attributed agent ("main" = orchestrator).
func (h *Handler) statsSkills(w http.ResponseWriter, r *http.Request) {
	dr, err := parseRange(r)
	if err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusBadRequest)
		return
	}
	agentFilter := r.URL.Query().Get("agent")
	pf, pargs := scopeFilter(r)
	rows, err := h.DB.Query(`
		SELECT json_extract(e.payload, '$.input.skill'), COALESCE(e.status, ''), e.duration_ms,
		       COALESCE(pe.type, ''), json_extract(pe.payload, '$.subagent_type')
		  FROM events e
		  JOIN sessions s ON s.id = e.session_id
		  JOIN projects p ON p.id = s.project_id
		  LEFT JOIN events pe ON pe.id = e.parent_event_id
		 WHERE e.type = 'skill_use'
		   AND json_extract(e.payload, '$.input.skill') IS NOT NULL
		   AND e.ts >= ? AND e.ts < ? AND p.archived = 0`+pf,
		append([]any{dr.start, dr.end}, pargs...)...)
	if err != nil {
		writeErr(w, err)
		return
	}
	defer rows.Close()

	type agg struct {
		calls, errors, denied int64
		durations             []int64
		agents                map[string]*toolAgentDTO
	}
	acc := map[string]*agg{}
	seenAgents := map[string]bool{}
	for rows.Next() {
		var skill, status, parentType string
		var durMs sql.NullInt64
		var subType sql.NullString
		if err := rows.Scan(&skill, &status, &durMs, &parentType, &subType); err != nil {
			writeErr(w, err)
			return
		}
		agent := "main"
		if parentType == "subagent_start" && subType.Valid && subType.String != "" {
			agent = normAgentType(subType.String)
		}
		seenAgents[agent] = true
		if agentFilter != "" && agent != agentFilter {
			continue
		}
		a := acc[skill]
		if a == nil {
			a = &agg{agents: map[string]*toolAgentDTO{}}
			acc[skill] = a
		}
		a.calls++
		switch status {
		case "error":
			a.errors++
		case "denied":
			a.denied++
		}
		if durMs.Valid {
			a.durations = append(a.durations, durMs.Int64)
		}
		ag := a.agents[agent]
		if ag == nil {
			ag = &toolAgentDTO{Agent: agent}
			a.agents[agent] = ag
		}
		ag.Calls++
		if status == "error" {
			ag.Errors++
		}
	}
	if err := rows.Err(); err != nil {
		writeErr(w, err)
		return
	}

	out := skillsDTO{
		From:   dr.days[0],
		To:     dr.days[len(dr.days)-1],
		Skills: make([]skillStatDTO, 0, len(acc)),
		Agents: sortedAgents(seenAgents),
	}
	for skill, a := range acc {
		ss := skillStatDTO{
			Skill: skill, Calls: a.calls, Errors: a.errors, Denied: a.denied,
			Agents: make([]toolAgentDTO, 0, len(a.agents)),
		}
		if n := len(a.durations); n > 0 {
			sort.Slice(a.durations, func(i, j int) bool { return a.durations[i] < a.durations[j] })
			var sum int64
			for _, d := range a.durations {
				sum += d
			}
			avg := float64(sum) / float64(n)
			ss.AvgMs = &avg
			idx := (n*95 + 99) / 100 // ceil(0.95 × n), 1-based
			p95 := a.durations[idx-1]
			ss.P95Ms = &p95
		}
		for _, ag := range a.agents {
			ss.Agents = append(ss.Agents, *ag)
		}
		sort.Slice(ss.Agents, func(i, j int) bool {
			if ss.Agents[i].Calls != ss.Agents[j].Calls {
				return ss.Agents[i].Calls > ss.Agents[j].Calls
			}
			return ss.Agents[i].Agent < ss.Agents[j].Agent
		})
		out.Skills = append(out.Skills, ss)
	}
	sort.Slice(out.Skills, func(i, j int) bool {
		if out.Skills[i].Calls != out.Skills[j].Calls {
			return out.Skills[i].Calls > out.Skills[j].Calls
		}
		return out.Skills[i].Skill < out.Skills[j].Skill
	})
	rolled, err := h.hasRolledUpDays(dr.days[0], dr.days[len(dr.days)-1], pf, pargs)
	if err != nil {
		writeErr(w, err)
		return
	}
	out.Approx = rolled
	writeJSON(w, out, nil)
}
