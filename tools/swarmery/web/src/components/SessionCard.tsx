import { useNavigate } from 'react-router-dom';
import type { Session } from '../api/types';
import { fmtSpan, fmtTime } from '../lib/format';
import { sessionState, useNowMs, type SessionState } from '../lib/sessionState';
import { KillButton, killSlotKind } from './KillButton';
import { OUTCOME_GLYPH } from './OutcomePicker';
import { ProjectName } from './ProjectName';
import { ProcBadge } from './ProcBadge';
import { StopButton } from './StopButton';
import { TaskChip } from './TaskChip';

function meta(session: Session): string {
  const parts: string[] = [];
  if (session.model !== null) parts.push(session.model);
  if (session.gitBranch !== null) parts.push(session.gitBranch);
  parts.push(
    session.endedAt !== null
      ? `ended ${fmtTime(session.endedAt)}`
      : `started ${fmtTime(session.startedAt)}`,
  );
  return parts.join(' · ');
}

/* ----- Canvas visual bucket — the UI speaks the tri-state (running/stuck/
 * done, lib/sessionState.ts) plus two display nuances kept from the existing
 * product surface: "waiting" (a session mid-approval must stay visible) and
 * "error" (killed rows keep their red accent). ----- */
type CanvasTone = 'active' | 'waiting' | 'stuck' | 'error' | 'done';

/** Tri-state → tone; waiting_approval and killed stay visible as nuances. */
function toneOf(s: Session, nowMs: number): CanvasTone {
  if (s.status === 'waiting_approval') return 'waiting';
  const state: SessionState = sessionState(s, nowMs);
  if (state === 'running') return 'active';
  if (state === 'stuck') return 'stuck';
  return s.status === 'killed' ? 'error' : 'done';
}

/* Live sessions get a status-tinted hairline (Redesign "Active now" card). */
const CARD_BORDERS: Partial<Record<CanvasTone, string>> = {
  active: 'border-green/25 hover:border-green/55',
  waiting: 'border-amber/35 hover:border-amber/70',
  stuck: 'border-amber/35 hover:border-amber/70',
};

const CANVAS_LABEL: Record<CanvasTone, string> = {
  active: 'working',
  waiting: 'waiting',
  stuck: 'stuck',
  error: 'killed',
  done: 'done',
};

const CANVAS_CHIP_STYLE: Record<CanvasTone, string> = {
  active: 'border-green/40 text-green',
  waiting: 'border-amber/40 text-amber',
  stuck: 'border-amber/40 text-amber',
  error: 'border-red/40 text-red',
  done: 'border-line-strong text-ink-dim',
};

/** Chip suffix: stuck shows QUIET TIME (silence since last transcript
 * activity), not session age — `working · 17 h 32 min` was the lie this
 * replaces. Everything else keeps the session span. */
function chipSuffix(session: Session, tone: CanvasTone): string {
  if (tone === 'stuck') {
    return `quiet ${fmtSpan(session.endedAt ?? session.startedAt, null)}`;
  }
  return fmtSpan(session.startedAt, session.endedAt);
}

/** Row status dot (Canvas §3a): only LIVE sessions carry a marker — a hollow
 * colour ring for active/error/waiting/stuck. done renders an empty span so
 * the grid column stays aligned without a resting-state dot. */
function RowDot({ tone }: { tone: CanvasTone }): JSX.Element {
  if (tone === 'active') {
    return <span className="inline-block h-2 w-2 shrink-0 animate-pulse-dot rounded-full border-2 border-green" />;
  }
  if (tone === 'error') {
    return <span className="inline-block h-2 w-2 shrink-0 rounded-full border-2 border-red" />;
  }
  if (tone === 'waiting' || tone === 'stuck') {
    return <span className="inline-block h-2 w-2 shrink-0 rounded-full border-2 border-amber" />;
  }
  return <span className="inline-block h-2 w-2 shrink-0" />;
}

/** Right-justified status chip (Canvas §3e): "working · 3h43m" / "stuck · quiet 42 min" / plain span. */
function RowChip({ tone, suffix }: { tone: CanvasTone; suffix: string }): JSX.Element {
  return (
    <span
      className={`justify-self-end rounded-full border px-[9px] py-0.5 font-mono text-[10.5px] whitespace-nowrap ${CANVAS_CHIP_STYLE[tone]}`}
    >
      {tone === 'active' || tone === 'stuck' || tone === 'error'
        ? `${CANVAS_LABEL[tone]} · ${suffix}`
        : suffix}
    </span>
  );
}

export function SessionCard({
  session,
  now = null,
  flat = false,
}: {
  session: Session;
  /** Live "now: <last action>" line, fed by event_appended WS messages. */
  now?: string | null;
  /** Row inside a grouped list card (no own border — hover fill instead). */
  flat?: boolean;
}): JSX.Element {
  const navigate = useNavigate();
  const nowMs = useNowMs();
  const tone = toneOf(session, nowMs);
  const liveNow = now !== null && (tone === 'active' || tone === 'waiting');
  const goToDetail = (): void => { navigate(`/sessions/${session.id}`); };

  /* Action slot: stuck rows with a confirmed-alive process keep the hard
   * Kill; any other live tone offers the graceful Stop (no PID needed);
   * done rows keep KillButton's existing 'exited' tag when a PID is known. */
  const action: JSX.Element | null =
    tone === 'stuck' && killSlotKind(session) === 'killable' ? (
      <KillButton session={session} />
    ) : tone === 'active' || tone === 'waiting' || tone === 'stuck' ? (
      <StopButton session={session} />
    ) : session.procPid != null ? (
      <KillButton session={session} />
    ) : null;

  /* Stacked card — standalone cards and the <900px rows inside day groups. */
  const card = (
    <>
      <div className="flex items-center gap-2">
        <RowDot tone={tone} />
        <ProjectName
          name={session.projectName}
          slug={session.projectSlug}
          className="min-w-0 flex-1 truncate font-mono text-[11px]"
        />
        <ProcBadge session={session} />
        {session.outcome != null && (
          <span
            role="img"
            aria-label={session.outcome}
            title={session.outcome}
            className={`font-mono text-[11px] ${OUTCOME_GLYPH[session.outcome].className}`}
          >
            {OUTCOME_GLYPH[session.outcome].glyph}
          </span>
        )}
        <RowChip tone={tone} suffix={chipSuffix(session, tone)} />
      </div>
      <div className="mt-px mb-[3px] truncate text-[13.5px] font-semibold">
        {session.title ?? session.sessionUuid}
      </div>
      <div className="truncate font-mono text-[11px] text-ink-dim">{meta(session)}</div>
      {session.taskExternalId != null && (
        <div className="mt-[3px] flex min-w-0">
          <TaskChip
            externalId={session.taskExternalId}
            linkSource={session.taskLinkSource}
            confidence={session.taskConfidence}
          />
        </div>
      )}
      {liveNow && (
        <div className="mt-[3px] truncate font-mono text-[10.5px] text-green">now: {now}</div>
      )}
      {action !== null && (
        <div className="mt-[3px] flex" onClick={(e) => e.stopPropagation()}>
          {action}
        </div>
      )}
    </>
  );

  /* Navigation via div+useNavigate instead of <Link> so that the action
   * buttons' stopPropagation reliably blocks navigation — <a> tags intercept
   * clicks at the browser level before React's synthetic event system can
   * stop them. */
  if (!flat) {
    return (
      <div
        role="link"
        tabIndex={0}
        onClick={goToDetail}
        onKeyDown={(e) => { if (e.key === 'Enter') goToDetail(); }}
        className={`mb-2.5 block cursor-pointer rounded-xl border bg-surface px-3.5 py-[11px] transition-colors focus-visible:outline-2 focus-visible:outline-brand ${
          CARD_BORDERS[tone] ?? 'border-line hover:border-ink-dim/50'
        }`}
      >
        {card}
      </div>
    );
  }

  /* Flat rows: mobile keeps the stacked card; ≥900px renders the Canvas
   * 5-column row (Canvas.dc.html §Sessions: dot / project / title+why /
   * model / status chip). Branch + start-time drop from their own columns
   * on desktop — they fold into the meta line under the title, same as the
   * stacked mobile card, so no data is lost, only re-laid-out. */
  return (
    <div
      role="link"
      tabIndex={0}
      onClick={goToDetail}
      onKeyDown={(e) => { if (e.key === 'Enter') goToDetail(); }}
      className="block cursor-pointer transition-colors hover:bg-surface focus-visible:outline-2 focus-visible:-outline-offset-2 focus-visible:outline-brand"
    >
      <div className="px-3.5 py-[11px] desk:hidden">{card}</div>
      <div className="hidden grid-cols-[15px_130px_minmax(0,1fr)_150px_90px] items-center gap-3.5 px-1 py-3 desk:grid">
        <span className="flex justify-center">
          <RowDot tone={tone} />
        </span>
        <span className="flex min-w-0 items-center">
          <ProjectName
            name={session.projectName}
            slug={session.projectSlug}
            className="truncate font-mono text-[11px]"
          />
        </span>
        <span className="min-w-0">
          <span className="flex min-w-0 items-baseline gap-1.5">
            <span
              className={`min-w-0 truncate text-[14px] font-semibold ${
                session.title === null ? 'font-normal text-ink-faint italic' : 'text-ink'
              }`}
            >
              {session.title ?? '(untitled session)'}
            </span>
            {session.outcome != null && (
              <span
                role="img"
                aria-label={session.outcome}
                title={session.outcome}
                className={`shrink-0 font-mono text-[11px] ${OUTCOME_GLYPH[session.outcome].className}`}
              >
                {OUTCOME_GLYPH[session.outcome].glyph}
              </span>
            )}
          </span>
          <span className="mt-0.5 block truncate text-[12px] text-ink-dim">
            {liveNow ? `now: ${now}` : (session.why ?? meta(session))}
          </span>
          {(session.taskExternalId != null || action !== null) && (
            <span className="mt-[3px] flex min-w-0 items-center gap-1.5">
              {session.taskExternalId != null && (
                <TaskChip
                  externalId={session.taskExternalId}
                  linkSource={session.taskLinkSource}
                  confidence={session.taskConfidence}
                />
              )}
              <ProcBadge session={session} />
              {action !== null && (
                <span onClick={(e) => e.stopPropagation()}>{action}</span>
              )}
            </span>
          )}
        </span>
        <span className="truncate font-mono text-[11px] text-ink-faint">
          {session.model ?? '—'}
        </span>
        <RowChip tone={tone} suffix={chipSuffix(session, tone)} />
      </div>
    </div>
  );
}
