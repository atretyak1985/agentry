// Desktop (≥1280px) right rail of the session detail (Redesign): AGENTS with
// duration pills, SKILLS, and FILES CHANGED with +/− counts. Everything is
// derived client-side from the already-loaded detail — no extra API calls.
// Mobile keeps the SummaryChips strip instead.

import { useMemo } from 'react';
import type { Event, FileChange } from '../../api/types';
import { fmtDurationMs, fmtSpan } from '../../lib/format';
import { subagentDescription, subagentName } from '../../lib/payload';
import { deriveSkills } from './SummaryChips';

interface AgentPill {
  key: string;
  name: string;
  /** "4m 12s" for finished agents; null while still running. */
  duration: string | null;
  /** Task description → native tooltip. */
  title: string | null;
}

function deriveAgentPills(events: Event[]): AgentPill[] {
  const pills: AgentPill[] = [];
  for (const event of events) {
    if (event.type !== 'subagent_start') continue;
    const stop = events.find(
      (e) => e.type === 'subagent_stop' && e.parentEventId === event.id,
    );
    const duration =
      stop !== undefined
        ? fmtDurationMs(stop.durationMs ?? null) || fmtSpan(event.ts, stop.ts)
        : null;
    pills.push({
      key: `agent-${String(event.id)}`,
      name: subagentName(event),
      duration,
      title: subagentDescription(event),
    });
  }
  return pills;
}

function RailLabel({ tone, children }: { tone: string; children: string }): JSX.Element {
  return (
    <div className={`mb-2 font-mono text-[10.5px] tracking-[0.08em] uppercase ${tone}`}>
      {children}
    </div>
  );
}

export function DetailRail({
  events,
  fileChanges,
  onShowDiffs,
}: {
  events: Event[];
  fileChanges: FileChange[];
  onShowDiffs: () => void;
}): JSX.Element | null {
  const agents = useMemo(() => deriveAgentPills(events), [events]);
  const skills = useMemo(() => deriveSkills(events), [events]);

  if (agents.length === 0 && skills.length === 0 && fileChanges.length === 0) return null;

  return (
    <div className="min-w-0 wide:sticky wide:top-[76px]">
      {(agents.length > 0 || skills.length > 0) && (
        <div className="mb-2.5 rounded-xl border border-line bg-surface px-4 py-3.5">
          {agents.length > 0 && (
            <>
              <RailLabel tone="text-blue/70">agents</RailLabel>
              <div className="flex flex-wrap gap-1.5">
                {agents.map((agent) => (
                  <span
                    key={agent.key}
                    title={agent.title ?? undefined}
                    className="max-w-full truncate rounded-full border border-blue/30 bg-blue/10 px-[9px] py-0.5 font-mono text-[11px] text-blue"
                  >
                    <span aria-hidden="true">⬡ </span>
                    {agent.name}
                    {agent.duration !== null ? ` · ${agent.duration}` : ' · running'}
                  </span>
                ))}
              </div>
            </>
          )}
          {skills.length > 0 && (
            <>
              <div className={agents.length > 0 ? 'mt-3.5' : ''}>
                <RailLabel tone="text-amber/70">skills</RailLabel>
              </div>
              <div className="flex flex-wrap gap-1.5">
                {skills.map((name) => (
                  <span
                    key={name}
                    className="max-w-full truncate rounded-full border border-amber/30 bg-amber/10 px-[9px] py-0.5 font-mono text-[11px] text-amber"
                  >
                    <span aria-hidden="true">◈ </span>
                    {name}
                  </span>
                ))}
              </div>
            </>
          )}
        </div>
      )}

      {fileChanges.length > 0 && (
        <div className="rounded-xl border border-line bg-surface px-4 py-3.5">
          <div className="mb-1 flex items-baseline justify-between">
            <span className="font-mono text-[10.5px] tracking-[0.08em] text-ink-dim uppercase">
              files changed
            </span>
            <span className="font-mono text-[12px] font-bold text-ink">{fileChanges.length}</span>
          </div>
          {fileChanges.map((change) => (
            <button
              key={change.id}
              type="button"
              onClick={onShowDiffs}
              className="flex w-full items-center gap-2 border-b border-line-soft py-1.5 text-left font-mono text-[11px] transition-colors last:border-b-0 hover:bg-surface2/50"
            >
              <span className="min-w-0 flex-1 truncate text-left text-ink-3 [direction:rtl]">
                {change.filePath}
              </span>
              <span className="text-green">+{change.additions ?? 0}</span>
              <span className="text-red">−{change.deletions ?? 0}</span>
            </button>
          ))}
        </div>
      )}
    </div>
  );
}
