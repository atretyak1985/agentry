package console

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// View renders the model. It carries NO business logic — every value comes from
// model accessors computed in Update — so keeping it out of the unit tests costs
// no coverage of behavior (the reducers are fully tested). Layout: header block,
// then two side-by-side panes (Events | Approvals), then a hotkey bar.

var (
	styleHeader   = lipgloss.NewStyle().Bold(true)
	styleDown     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("9")) // red
	styleBar      = lipgloss.NewStyle().Faint(true)
	styleSelected = lipgloss.NewStyle().Reverse(true)
	stylePaneHdr  = lipgloss.NewStyle().Underline(true)
	styleFlash    = lipgloss.NewStyle().Foreground(lipgloss.Color("10")) // green

	tagColors = map[string]lipgloss.Color{
		"ingest":    lipgloss.Color("12"),
		"approvals": lipgloss.Color("11"),
		"dispatch":  lipgloss.Color("13"),
		"verify":    lipgloss.Color("14"),
		"routines":  lipgloss.Color("6"),
		"provision": lipgloss.Color("5"),
		"api":       lipgloss.Color("8"),
		"boot":      lipgloss.Color("10"),
	}
)

// View satisfies tea.Model.
func (m Model) View() string {
	if m.quitting {
		return ""
	}
	var b strings.Builder

	// Header (status block or down banner) + WS liveness chip.
	if m.statusErr != "" {
		b.WriteString(styleDown.Render(m.headerText()))
	} else {
		b.WriteString(styleHeader.Render(m.headerText()))
	}
	b.WriteString("   " + m.wsChip() + "\n\n")

	// Two panes side by side, sized to the terminal when known.
	paneW := 40
	paneH := 16
	if m.width > 20 {
		paneW = (m.width - 3) / 2
	}
	if m.height > 12 {
		paneH = m.height - 8
	}
	left := m.renderEvents(paneW, paneH)
	right := m.renderApprovals(paneW, paneH)
	b.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, left, "  ", right))
	b.WriteString("\n")

	if m.flash != "" {
		b.WriteString(styleFlash.Render(m.flash) + "\n")
	}
	b.WriteString(styleBar.Render(m.hotkeyBar()))
	return b.String()
}

func (m Model) wsChip() string {
	if m.wsConnected {
		return lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Render("● live")
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color("11")).Render("○ reconnecting…")
}

func (m Model) renderEvents(w, h int) string {
	title := "Events"
	if m.tagFilter != "" {
		title += " [" + m.tagFilter + "]"
	}
	lines := m.visibleFeed()
	// Show the last h lines (newest at the bottom).
	if len(lines) > h {
		lines = lines[len(lines)-h:]
	}
	var body strings.Builder
	for _, l := range lines {
		tag := l.tag
		if c, ok := tagColors[l.tag]; ok {
			tag = lipgloss.NewStyle().Foreground(c).Render(l.tag)
		}
		line := fmt.Sprintf("[%s] %s", tag, l.text)
		if !m.wrap && lipgloss.Width(line) > w {
			line = truncate(line, w)
		}
		body.WriteString(line + "\n")
	}
	return stylePaneHdr.Render(title) + "\n" + body.String()
}

func (m Model) renderApprovals(w, h int) string {
	p := m.pending()
	var body strings.Builder
	for i, a := range p {
		tool, ctx := approvalContext(a)
		line := fmt.Sprintf("#%d %s", a.ID, tool)
		if i == m.selected {
			line = styleSelected.Render("▶ " + line)
			// Selected row expands to show the gated action context.
			if ctx != "" {
				line += "\n    " + truncate(ctx, w)
			}
		} else {
			line = "  " + line
		}
		body.WriteString(line + "\n")
	}
	if len(p) == 0 {
		body.WriteString("(no pending approvals)\n")
	}
	return stylePaneHdr.Render(fmt.Sprintf("Approvals (%d)", len(p))) + "\n" + body.String()
}

func (m Model) hotkeyBar() string {
	return "[↑↓] select  [y] approve  [n] deny  [p] pause  [o] dashboard  [w] wrap  [f] filter  [q] quit"
}

// approvalContext extracts a human tool + args/cwd summary from RequestJSON for
// the selected-row detail. Best-effort: raw JSON on parse failure.
func approvalContext(a Approval) (tool, ctx string) {
	tool = a.ToolName
	raw := strings.TrimSpace(a.RequestJSON)
	if raw == "" {
		return tool, ""
	}
	// Keep it simple + robust: show a trimmed single-line form of the payload.
	oneLine := strings.Join(strings.Fields(raw), " ")
	return tool, oneLine
}

func truncate(s string, w int) string {
	if w <= 1 || lipgloss.Width(s) <= w {
		return s
	}
	// Trim by runes to the width budget, leaving room for an ellipsis.
	r := []rune(s)
	if len(r) > w-1 {
		r = r[:w-1]
	}
	return string(r) + "…"
}
