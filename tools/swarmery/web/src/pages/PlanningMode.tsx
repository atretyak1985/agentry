// Planning Mode (fusion phase 8): "describe what you want to build" → a headless
// planner run turns the idea into a structured plan in the private workspace,
// which the user activates into board tasks.
//
// Three states:
//  · idle   — headline, textarea, 3 example chips, Start.
//  · active — the linked planner session (live), the planner's latest reply text
//             (its clarifying questions arrive here — the phase-8 spike proved
//             AskUserQuestion does NOT fire the permission hook under `claude -p`,
//             so the planner asks as reply TEXT), a composer that answers via the
//             existing session-resume chat, plus any inline pending approvals
//             questions for the run (defensive — rendered with the shared
//             QuestionForm if a future Claude build DOES surface them).
//  · done   — a new workspace task row for this project appeared (settle-poll) →
//             "Plan ready" + link + Activate (creates a Triage board task).
//
// Frozen WS bus: reuses session_updated + task_updated + a settle-poll; no new
// message types.

import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { Link } from 'react-router-dom';
import type {
  PermissionRequest,
  PlanningStatus,
  TaskSummary,
  Turn,
  WSMessage,
} from '../api/types';
import {
  cancelPlanning,
  fetchApprovals,
  fetchPlanning,
  fetchSession,
  fetchTasks,
  resolveApproval,
  sendSessionMessage,
  startPlanning,
} from '../api';
import { activatePlan } from '../api/activatePlan';
import { questionsOf } from '../lib/approvals';
import { fmtAgo } from '../lib/format';
import { useLiveUpdates } from '../lib/ws';
import { useProjectWorkspace } from '../workspace/ProjectContext';
import { Card, Empty, ErrorBox, Loading } from '../components/ui';
import { QuestionForm } from '../components/QuestionForm';

const EXAMPLE_IDEAS = [
  'Add a dark-mode toggle to the settings page, persisted per user.',
  'Introduce rate limiting on the public API with a per-key budget.',
  'Migrate the auth middleware off the deprecated session store.',
];

// Settle-poll cadence for the "plan ready" workspace task (wsingest rescans on a
// 60s cadence, so we poll a little faster while a run is active).
const PLAN_POLL_MS = 15_000;

/** The planner's latest reply: the last assistant turn with non-empty text. */
function latestAssistantText(turns: readonly Turn[]): string {
  for (let i = turns.length - 1; i >= 0; i--) {
    const t = turns[i];
    if (t !== undefined && t.role === 'assistant' && t.text !== null && t.text.trim() !== '') {
      return t.text;
    }
  }
  return '';
}

export function PlanningMode(): JSX.Element {
  const { projectId, project, slug, loading } = useProjectWorkspace();

  const [status, setStatus] = useState<PlanningStatus | null>(null);
  const [idea, setIdea] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  // The planner session's latest reply text (clarifying questions land here).
  const [replyText, setReplyText] = useState('');
  // Inline pending approvals questions for the run (defensive path).
  const [pending, setPending] = useState<PermissionRequest[]>([]);
  // The workspace task row that appears once the plan dir is written.
  const [plan, setPlan] = useState<TaskSummary | null>(null);
  const [activated, setActivated] = useState<{ id: string } | null>(null);
  // Answer composer (session-resume chat) for the planner's clarifying questions.
  const [answer, setAnswer] = useState('');

  // The instant the current run started, so a workspace task is only treated as
  // THIS plan when it appears at/after it (avoids matching a pre-existing row).
  const runStartedRef = useRef<number>(0);
  const aliveRef = useRef(true);

  const active = status?.active ?? false;
  const sessionUuid = status?.sessionUuid ?? '';
  const sessionId = status?.sessionId ?? null;

  const loadStatus = useCallback((): void => {
    if (projectId === null) return;
    fetchPlanning(projectId)
      .then((s) => {
        if (!aliveRef.current) return;
        setStatus(s);
      })
      .catch((e: unknown) => {
        if (!aliveRef.current) return;
        setError(e instanceof Error ? e.message : String(e));
      });
  }, [projectId]);

  useEffect(() => {
    aliveRef.current = true;
    loadStatus();
    return () => {
      aliveRef.current = false;
    };
  }, [loadStatus]);

  // While active, keep the run's start time so plan detection is scoped.
  useEffect(() => {
    if (active && runStartedRef.current === 0) {
      const startedMs = status?.startedAt != null ? new Date(status.startedAt).getTime() : Date.now();
      runStartedRef.current = startedMs;
    }
    if (!active) {
      runStartedRef.current = 0;
    }
  }, [active, status?.startedAt]);

  // Poll the planner session's transcript for the latest reply text + its
  // pending approvals questions, while a run is active.
  useEffect(() => {
    if (!active || sessionUuid === '') {
      setReplyText('');
      setPending([]);
      return undefined;
    }
    let disposed = false;
    const poll = (): void => {
      fetchSession(sessionUuid)
        .then((d) => {
          if (disposed) return;
          setReplyText(latestAssistantText(d.turns));
        })
        .catch(() => {
          /* transcript not ingested yet — the next tick retries */
        });
      // Inline pending questions for the run (defensive — normally empty under -p).
      fetchApprovals('pending')
        .then((reqs) => {
          if (disposed) return;
          setPending(
            reqs.filter(
              (r) =>
                r.toolName === 'AskUserQuestion' && sessionId !== null && r.sessionId === sessionId,
            ),
          );
        })
        .catch(() => {
          /* approvals unavailable — ignore */
        });
    };
    poll();
    const t = window.setInterval(poll, 4_000);
    return () => {
      disposed = true;
      window.clearInterval(t);
    };
  }, [active, sessionUuid, sessionId]);

  // Settle-poll for the plan dir → a workspace task row for this project that
  // appeared at/after the run start. Runs while active or just after (until a
  // plan is found), then stops.
  useEffect(() => {
    if (projectId === null || slug === '') return undefined;
    if (plan !== null) return undefined; // already found
    let disposed = false;
    const poll = (): void => {
      fetchTasks()
        .then((tasks) => {
          if (disposed) return;
          const mine = tasks
            .filter((t) => t.projectSlug === slug)
            .filter((t) => {
              const started = t.startedAt != null ? new Date(t.startedAt).getTime() : 0;
              return runStartedRef.current === 0 || started >= runStartedRef.current - 60_000;
            })
            .sort((a, b) => (b.startedAt ?? '').localeCompare(a.startedAt ?? ''));
          const newest = mine[0];
          if (newest !== undefined) setPlan(newest);
        })
        .catch(() => {
          /* tasks unavailable — retry next tick */
        });
    };
    // Only poll once a run has been started this session (runStartedRef set) or is active.
    if (runStartedRef.current !== 0 || active) {
      poll();
      const t = window.setInterval(poll, PLAN_POLL_MS);
      return () => {
        disposed = true;
        window.clearInterval(t);
      };
    }
    return undefined;
  }, [projectId, slug, active, plan]);

  // Live nudges: session_updated / task_updated → refresh status + plan.
  const onMessage = useCallback(
    (msg: WSMessage): void => {
      if (msg.type === 'session_updated' || msg.type === 'task_updated') {
        loadStatus();
      }
    },
    [loadStatus],
  );
  useLiveUpdates(onMessage, loadStatus);

  const start = (): void => {
    if (projectId === null || idea.trim() === '') return;
    setBusy(true);
    setError(null);
    setPlan(null);
    setActivated(null);
    runStartedRef.current = Date.now();
    startPlanning(projectId, idea.trim())
      .then((res) => {
        if (!aliveRef.current) return;
        setStatus({ active: true, sessionUuid: res.sessionUuid, sessionId: null, startedAt: new Date().toISOString() });
      })
      .catch((e: unknown) => {
        if (!aliveRef.current) return;
        setError(e instanceof Error ? e.message : String(e));
        runStartedRef.current = 0;
      })
      .finally(() => {
        if (aliveRef.current) setBusy(false);
      });
  };

  const cancel = (): void => {
    if (projectId === null) return;
    setBusy(true);
    cancelPlanning(projectId)
      .then(() => aliveRef.current && loadStatus())
      .catch((e: unknown) => aliveRef.current && setError(e instanceof Error ? e.message : String(e)))
      .finally(() => aliveRef.current && setBusy(false));
  };

  const submitAnswer = (text: string): void => {
    if (sessionId === null || text.trim() === '') return;
    setBusy(true);
    sendSessionMessage(sessionId, text.trim())
      .then(() => {
        if (!aliveRef.current) return;
        setAnswer('');
        setReplyText(''); // the planner will reply again; clear the prior prompt
      })
      .catch((e: unknown) => aliveRef.current && setError(e instanceof Error ? e.message : String(e)))
      .finally(() => aliveRef.current && setBusy(false));
  };

  const answerApproval = (requestId: number, answers: Record<string, string | string[]>): void => {
    setBusy(true);
    resolveApproval(requestId, 'answer', undefined, answers)
      .then(() => aliveRef.current && setPending((prev) => prev.filter((r) => r.id !== requestId)))
      .catch((e: unknown) => aliveRef.current && setError(e instanceof Error ? e.message : String(e)))
      .finally(() => aliveRef.current && setBusy(false));
  };

  const runActivate = (): void => {
    if (plan === null || projectId === null) return;
    setBusy(true);
    activatePlan({ task: plan, projectId, idea })
      .then((task) => aliveRef.current && setActivated({ id: task.externalId }))
      .catch((e: unknown) => aliveRef.current && setError(e instanceof Error ? e.message : String(e)))
      .finally(() => aliveRef.current && setBusy(false));
  };

  const projectLabel = useMemo(() => project?.name ?? project?.slug ?? slug, [project, slug]);

  if (loading && status === null) {
    return (
      <div className="px-4 pt-6 pb-10 desk:px-8">
        <Loading label="planning…" />
      </div>
    );
  }
  if (projectId === null) {
    return (
      <div className="px-4 pt-6 pb-10 desk:px-8">
        <Empty>unknown project — pick one from the switcher</Empty>
      </div>
    );
  }

  return (
    <div className="min-w-0 px-4 pt-6 pb-10 desk:px-8 desk:pt-8 desk:pb-[60px]">
      <h1 className="font-display text-[26px] font-medium tracking-[-0.01em] desk:text-[30px]">
        Transform your idea into a plan
      </h1>
      <p className="mt-1.5 max-w-[70ch] text-[13px] text-ink-dim">
        Describe what you want to build for{' '}
        <span className="font-mono text-ink">{projectLabel}</span>. A planner session asks any
        clarifying questions, then writes a structured plan you can activate into board tasks.
      </p>

      {error !== null && (
        <div className="mt-3">
          <ErrorBox message={error} onRetry={() => setError(null)} />
        </div>
      )}

      {/* IDLE — idea intake */}
      {!active && plan === null && (
        <div className="mt-5 max-w-[80ch]">
          <textarea
            value={idea}
            onChange={(e) => setIdea(e.target.value)}
            rows={5}
            placeholder="e.g. Add a bulk-export button to the reports page that streams a CSV…"
            aria-label="describe what you want to build"
            className="w-full resize-y rounded-xl border border-line bg-field px-3.5 py-3 text-[13.5px] leading-relaxed text-ink transition-colors outline-none placeholder:text-ink-faint focus:border-brand/50"
          />
          <div className="mt-2 flex flex-wrap gap-1.5">
            {EXAMPLE_IDEAS.map((ex) => (
              <button
                key={ex}
                type="button"
                onClick={() => setIdea(ex)}
                className="rounded-full border border-line px-2.5 py-1 font-mono text-[10.5px] text-ink-dim transition-colors hover:border-line-strong hover:text-ink"
              >
                {ex.length > 52 ? `${ex.slice(0, 52)}…` : ex}
              </button>
            ))}
          </div>
          <button
            type="button"
            disabled={busy || idea.trim() === ''}
            onClick={start}
            className="mt-3 rounded-lg border border-brand/50 bg-brand/12 px-4 py-2 text-[13px] font-semibold text-brand transition-colors hover:bg-brand/20 disabled:opacity-50"
          >
            {busy ? 'starting…' : 'Start planning'}
          </button>
        </div>
      )}

      {/* ACTIVE — the planner run */}
      {active && (
        <Card>
          <div className="flex flex-wrap items-center gap-2.5">
            <span className="inline-block h-[7px] w-[7px] shrink-0 animate-pulse rounded-full bg-brand" aria-hidden="true" />
            <span className="text-[13px] font-semibold text-ink">Planner running</span>
            {status?.startedAt != null && (
              <span className="font-mono text-[10.5px] text-ink-faint">started {fmtAgo(status.startedAt)}</span>
            )}
            {sessionUuid !== '' && (
              <Link
                to={`/sessions/${sessionUuid}`}
                className="font-mono text-[11px] text-ink-dim transition-colors hover:text-brand"
              >
                open session →
              </Link>
            )}
            <button
              type="button"
              disabled={busy}
              onClick={cancel}
              className="ml-auto rounded-lg border border-red/40 px-3 py-1 font-mono text-[11px] text-red transition-colors hover:bg-red/10 disabled:opacity-50"
            >
              cancel
            </button>
          </div>

          {/* Inline pending approvals questions (defensive path) */}
          {pending.map((req) => {
            const questions = questionsOf(req);
            if (questions === null) return null;
            return (
              <div key={req.id} className="mt-3 rounded-[10px] border border-amber/30 px-3 py-2.5">
                <div className="mb-2 font-mono text-[10.5px] tracking-[0.1em] text-amber uppercase">
                  the planner is asking
                </div>
                <QuestionForm
                  questions={questions}
                  idNamespace={`plan-ask-${String(req.id)}`}
                  busy={busy}
                  onSubmit={(answers) => answerApproval(req.id, answers)}
                />
              </div>
            );
          })}

          {/* The planner's latest reply text (clarifying questions arrive here) */}
          {replyText !== '' ? (
            <div className="mt-3">
              <div className="mb-1.5 font-mono text-[10.5px] tracking-[0.1em] text-ink-faint uppercase">
                planner
              </div>
              <div className="max-h-72 overflow-y-auto rounded-lg border border-line bg-bg px-3 py-2.5 text-[12.5px] leading-relaxed whitespace-pre-wrap text-ink-2">
                {replyText}
              </div>
              <form
                className="mt-2 flex flex-wrap gap-2"
                onSubmit={(e) => {
                  e.preventDefault();
                  submitAnswer(answer);
                }}
              >
                <input
                  type="text"
                  value={answer}
                  onChange={(e) => setAnswer(e.target.value)}
                  placeholder="answer the planner…"
                  aria-label="answer the planner"
                  disabled={sessionId === null}
                  className="min-w-0 flex-1 basis-[240px] rounded-lg border border-line bg-field px-2.5 py-[7px] font-mono text-[11.5px] text-ink transition-colors outline-none placeholder:text-ink-faint focus:border-brand/50 disabled:opacity-50"
                />
                <button
                  type="submit"
                  disabled={busy || sessionId === null || answer.trim() === ''}
                  className="rounded-lg border border-brand/45 bg-brand/12 px-3.5 py-1.5 font-mono text-[11.5px] font-semibold text-brand transition-colors hover:bg-brand/20 disabled:opacity-50"
                >
                  reply
                </button>
              </form>
              {sessionId === null && (
                <div className="mt-1 font-mono text-[10px] text-ink-faint">
                  waiting for the session to register before you can reply…
                </div>
              )}
            </div>
          ) : (
            <div className="mt-3 font-mono text-[11px] text-ink-dim">
              waiting for the planner’s first reply…
            </div>
          )}
        </Card>
      )}

      {/* DONE — plan ready */}
      {plan !== null && (
        <Card>
          <div className="flex flex-wrap items-center gap-2.5">
            <span className="inline-block h-[7px] w-[7px] shrink-0 rounded-full bg-green" aria-hidden="true" />
            <span className="text-[13px] font-semibold text-ink">Plan ready</span>
            <Link
              to={`/p/${slug}/plans`}
              className="font-mono text-[11px] text-ink-dim transition-colors hover:text-brand"
            >
              {plan.title}
            </Link>
            {plan.startedAt != null && (
              <span className="font-mono text-[10.5px] text-ink-faint">{fmtAgo(plan.startedAt)}</span>
            )}
          </div>
          <div className="mt-2 font-mono text-[11px] text-ink-dim">
            plan tracked as <span className="text-ink">{plan.externalId}</span> — activate it to create
            a board task the dispatcher can pick up.
          </div>
          {activated === null ? (
            <button
              type="button"
              disabled={busy}
              onClick={runActivate}
              className="mt-3 rounded-lg border border-green/45 bg-green/12 px-4 py-2 text-[13px] font-semibold text-green transition-colors hover:bg-green/20 disabled:opacity-50"
            >
              {busy ? 'activating…' : 'Activate plan'}
            </button>
          ) : (
            <div className="mt-3 flex flex-wrap items-center gap-2 text-[12.5px] text-ink-2">
              <span className="font-mono text-green">✓ activated</span>
              created board task <span className="font-mono text-ink">{activated.id}</span> —
              <Link to={`/p/${slug}/board`} className="text-brand hover:underline">
                open board →
              </Link>
            </div>
          )}
        </Card>
      )}
    </div>
  );
}
