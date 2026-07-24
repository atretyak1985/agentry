// Dispatcher pause-scope key (fusion phase 3/4): mirrors the Go
// dispatch.ProjectScope helper ("project:<id>") so the status bar can test
// membership in GET /api/dispatch → pausedScopes. Keep in lockstep with
// internal/dispatch/service.go.

export function projectScopeKey(projectId: number): string {
  return `project:${String(projectId)}`;
}
