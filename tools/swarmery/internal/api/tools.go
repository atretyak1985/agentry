package api

// Analytics uplift: GET /api/stats/tools — per-tool call/error/denied counts
// and duration stats (avg + p95) over a local-day range, with a per-agent
// split for the expandable row in the Tools panel (web/src/pages/Analytics.tsx).
//
// p95 is computed in Go after fetching per-tool durations: SQLite ships no
// percentile aggregate without extensions, and a date-bounded range holds at
// most tens of thousands of events — a single fetch + sort is simpler and
// plenty fast.
//
// Agent attribution uses what the ingester actually stores (events.agent_id
// is never populated — see analytics.go's header): a sidechain tool event is
// parented (parent_event_id, adoptOrphanSidechainEvents) to its
// subagent_start, whose payload carries subagent_type; a NULL parent is the
// orchestrator ("main"). events.turn_id is deliberately NOT used — openToolCall
// zeroes it for sidechain events.

import (
	"database/sql"
	"net/http"
	"sort"
)

type toolAgentDTO struct {
	Agent  string `json:"agent"`
	Calls  int64  `json:"calls"`
	Errors int64  `json:"errors"`
}

// sortedAgents returns the attributed-agent set as a stable list — "main"
// (the orchestrator) first, then the rest alphabetically. Shared by the tools
// and skills panels for their agent-filter dropdown.
func sortedAgents(seen map[string]bool) []string {
	out := make([]string, 0, len(seen))
	for a := range seen {
		out = append(out, a)
	}
	sort.Slice(out, func(i, j int) bool {
		if (out[i] == "main") != (out[j] == "main") {
			return out[i] == "main"
		}
		return out[i] < out[j]
	})
	return out
}

type toolStatDTO struct {
	Tool   string         `json:"tool"`
	Calls  int64          `json:"calls"`
	Errors int64          `json:"errors"`
	Denied int64          `json:"denied"`
	AvgMs  *float64       `json:"avg_ms"` // nil when no call carried a duration
	P95Ms  *int64         `json:"p95_ms"` // nil when no call carried a duration
	Agents []toolAgentDTO `json:"agents"`
}

type toolsDTO struct {
	From  string        `json:"from"`
	To    string        `json:"to"`
	Tools []toolStatDTO `json:"tools"`
	// Agents lists every attributed agent seen in the range (NOT narrowed by
	// the ?agent= filter) — the option set for the panel's agent dropdown.
	Agents []string `json:"agents"`
	// Approx is true when the range overlaps pruned (rolled-up) days — daily
	// rollups carry no per-tool events, so the counts silently undercount
	// there. Same honesty rule as the timeseries `approx` badge.
	Approx bool `json:"approx"`
}

// GET /api/stats/tools?from&to&project&agent — project is the optional global
// scope (slug or id, resolved by scopeFilter). agent optionally narrows every
// row + column to a single attributed agent ("main" = orchestrator), so the
// whole table reflects that agent's tool usage.
func (h *Handler) statsTools(w http.ResponseWriter, r *http.Request) {
	dr, err := parseRange(r)
	if err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusBadRequest)
		return
	}
	agentFilter := r.URL.Query().Get("agent")
	pf, pargs := scopeFilter(r)
	rows, err := h.DB.Query(`
		SELECT e.tool_name, COALESCE(e.status, ''), e.duration_ms,
		       COALESCE(pe.type, ''), json_extract(pe.payload, '$.subagent_type')
		  FROM events e
		  JOIN sessions s ON s.id = e.session_id
		  JOIN projects p ON p.id = s.project_id
		  LEFT JOIN events pe ON pe.id = e.parent_event_id
		 WHERE e.tool_name IS NOT NULL
		   AND e.type IN ('tool_call', 'skill_use', 'subagent_start')
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
		var tool, status, parentType string
		var durMs sql.NullInt64
		var subType sql.NullString
		if err := rows.Scan(&tool, &status, &durMs, &parentType, &subType); err != nil {
			writeErr(w, err)
			return
		}
		agent := "main"
		if parentType == "subagent_start" && subType.Valid && subType.String != "" {
			agent = normAgentType(subType.String)
		}
		// The dropdown option set is the FULL agent list, so record it before
		// the ?agent= filter narrows the rows.
		seenAgents[agent] = true
		if agentFilter != "" && agent != agentFilter {
			continue
		}
		a := acc[tool]
		if a == nil {
			a = &agg{agents: map[string]*toolAgentDTO{}}
			acc[tool] = a
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

	out := toolsDTO{
		From:   dr.days[0],
		To:     dr.days[len(dr.days)-1],
		Tools:  make([]toolStatDTO, 0, len(acc)),
		Agents: sortedAgents(seenAgents),
	}
	for tool, a := range acc {
		ts := toolStatDTO{
			Tool: tool, Calls: a.calls, Errors: a.errors, Denied: a.denied,
			Agents: make([]toolAgentDTO, 0, len(a.agents)),
		}
		if n := len(a.durations); n > 0 {
			sort.Slice(a.durations, func(i, j int) bool { return a.durations[i] < a.durations[j] })
			var sum int64
			for _, d := range a.durations {
				sum += d
			}
			avg := float64(sum) / float64(n)
			ts.AvgMs = &avg
			idx := (n*95 + 99) / 100 // ceil(0.95 × n), 1-based
			p95 := a.durations[idx-1]
			ts.P95Ms = &p95
		}
		for _, ag := range a.agents {
			ts.Agents = append(ts.Agents, *ag)
		}
		sort.Slice(ts.Agents, func(i, j int) bool {
			if ts.Agents[i].Calls != ts.Agents[j].Calls {
				return ts.Agents[i].Calls > ts.Agents[j].Calls
			}
			return ts.Agents[i].Agent < ts.Agents[j].Agent
		})
		out.Tools = append(out.Tools, ts)
	}
	sort.Slice(out.Tools, func(i, j int) bool {
		if out.Tools[i].Calls != out.Tools[j].Calls {
			return out.Tools[i].Calls > out.Tools[j].Calls
		}
		return out.Tools[i].Tool < out.Tools[j].Tool
	})
	rolled, err := h.hasRolledUpDays(dr.days[0], dr.days[len(dr.days)-1], pf, pargs)
	if err != nil {
		writeErr(w, err)
		return
	}
	out.Approx = rolled
	writeJSON(w, out, nil)
}
