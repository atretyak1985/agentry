package notify

import (
	"database/sql"
	"fmt"
)

// SessionEvent builds the session_completed / session_error event for a
// session the status ticker just moved to 'completed'. errorCount is the
// caller's COUNT of the session's events with status='error' (the same
// predicate the stats endpoints use); > 0 flips the type to session_error.
func SessionEvent(db *sql.DB, sessionID int64, errorCount int) (Event, error) {
	var uuid, title, project string
	err := db.QueryRow(
		`SELECT s.session_uuid, COALESCE(s.title, ''), COALESCE(p.name, p.slug)
		 FROM sessions s JOIN projects p ON p.id = s.project_id
		 WHERE s.id = ?`, sessionID).Scan(&uuid, &title, &project)
	if err != nil {
		return Event{}, fmt.Errorf("load session %d: %w", sessionID, err)
	}
	label := title
	if label == "" {
		label = uuid
	}
	if errorCount > 0 {
		return Event{
			Type: EventSessionError, SessionID: sessionID, Project: project,
			Title: "Session finished with errors",
			Body:  fmt.Sprintf("%s — %s (%d error(s))", project, label, errorCount),
		}, nil
	}
	return Event{
		Type: EventSessionCompleted, SessionID: sessionID, Project: project,
		Title: "Session finished",
		Body:  fmt.Sprintf("%s — %s", project, label),
	}, nil
}
