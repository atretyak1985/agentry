// Typed client for the swarmery REST API (skeleton contract).

export interface Session {
  id: number;
  projectId: number;
  projectSlug: string;
  sessionUuid: string;
  model: string | null;
  gitBranch: string | null;
  cwd: string | null;
  status: 'active' | 'idle' | 'completed';
  startedAt: string;
  endedAt: string | null;
  title: string | null;
  source: string;
}

export interface Turn {
  id: number;
  seq: number;
  role: 'user' | 'assistant';
  messageId: string | null;
  startedAt: string;
  endedAt: string | null;
  tokensIn: number | null;
  tokensOut: number | null;
  tokensCacheRead: number | null;
  tokensCacheWrite: number | null;
  costUsd: number | null;
}

export interface Event {
  id: number;
  turnId: number | null;
  ts: string;
  type: string;
  toolName: string | null;
  parentEventId: number | null;
  status: string | null;
  durationMs: number | null;
  payload: unknown;
}

export interface FileChange {
  id: number;
  eventId: number;
  filePath: string;
  changeType: string;
  additions: number | null;
  deletions: number | null;
  diff: string | null;
  outOfScope: boolean;
}

export interface SessionDetail extends Session {
  turns: Turn[];
  events: Event[];
  fileChanges: FileChange[];
}

async function get<T>(path: string): Promise<T> {
  const res = await fetch(path);
  if (!res.ok) {
    throw new Error(`GET ${path}: ${res.status}`);
  }
  return (await res.json()) as T;
}

export const fetchSessions = (): Promise<Session[]> => get('/api/sessions');
export const fetchSession = (id: number): Promise<SessionDetail> => get(`/api/sessions/${id}`);
