// Desktop (≥1280px) right rail of the session detail (Redesign): AGENTS with
// duration pills, SKILLS, and FILES CHANGED aggregated per path (one row per
// file, +/− summed across all its edits, sorted by churn). Everything is
// derived client-side from the already-loaded detail — no extra API calls.
// Mobile keeps the SummaryChips strip instead.

import { useMemo } from 'react';
import type { Event, FileChange } from '../../api/types';
import { fmtDurationMs } from '../../lib/format';
import { subagentDescription, subagentName } from '../../lib/payload';
import { deriveSkills } from './SummaryChips';

interface AgentGroup {
  name: string;
  count: number;
  running: number;
  totalMs: number;
  /** "description — duration" per run → native tooltip. */
  titles: string[];
}

/** Group subagent runs by type: one pill per agent name with ×N and total time. */
function groupAgents(events: Event[]): AgentGroup[] {
  const byName = new Map<string, AgentGroup>();
  for (const event of events) {
    if (event.type !== 'subagent_start') continue;
    const stop = events.find(
      (e) => e.type === 'subagent_stop' && e.parentEventId === event.id,
    );
    const name = subagentName(event);
    const group = byName.get(name) ?? { name, count: 0, running: 0, totalMs: 0, titles: [] };
    group.count += 1;
    if (stop === undefined) group.running += 1;
    const ms = stop?.durationMs ?? null;
    if (ms !== null) group.totalMs += ms;
    const desc = subagentDescription(event);
    if (desc !== null) {
      group.titles.push(ms !== null ? `${desc} — ${fmtDurationMs(ms)}` : `${desc} — running`);
    }
    byName.set(name, group);
  }
  return [...byName.values()].sort((a, b) => b.count - a.count);
}

interface FileRow {
  path: string;
  additions: number;
  deletions: number;
}

/** One row per file path: +/− summed over all its file_change rows, sorted by total churn desc. */
function aggregateFileChanges(changes: FileChange[]): FileRow[] {
  const byPath = new Map<string, FileRow>();
  for (const change of changes) {
    const row = byPath.get(change.filePath) ?? {
      path: change.filePath,
      additions: 0,
      deletions: 0,
    };
    row.additions += change.additions ?? 0;
    row.deletions += change.deletions ?? 0;
    byPath.set(change.filePath, row);
  }
  return [...byPath.values()].sort(
    (a, b) => b.additions + b.deletions - (a.additions + a.deletions),
  );
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
  onShowDiffs: (path?: string) => void;
}): JSX.Element | null {
  const agents = useMemo(() => groupAgents(events), [events]);
  const skills = useMemo(() => deriveSkills(events), [events]);
  const files = useMemo(() => aggregateFileChanges(fileChanges), [fileChanges]);

  if (agents.length === 0 && skills.length === 0 && files.length === 0) return null;

  return (
    <div className="min-w-0">
      {(agents.length > 0 || skills.length > 0) && (
        <div className="mb-2.5 rounded-xl border border-line bg-surface px-4 py-3.5">
          {agents.length > 0 && (
            <>
              <RailLabel tone="text-blue/70">agents</RailLabel>
              <div className="flex flex-wrap gap-1.5">
                {agents.map((group) => (
                  <span
                    key={group.name}
                    title={group.titles.length > 0 ? group.titles.join('\n') : undefined}
                    className="max-w-full truncate rounded-full border border-blue/30 bg-blue/10 px-[9px] py-0.5 font-mono text-[11px] text-blue"
                  >
                    <span aria-hidden="true">⬡ </span>
                    {group.name}
                    {group.count > 1 ? ` ×${String(group.count)}` : ''}
                    {group.totalMs > 0 ? ` · ${fmtDurationMs(group.totalMs)}` : ''}
                    {group.running > 0 ? ' · running' : ''}
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

      {files.length > 0 && (
        <div className="rounded-xl border border-line bg-surface px-4 py-3.5">
          <div className="mb-1 flex items-baseline justify-between">
            <span className="font-mono text-[10.5px] tracking-[0.08em] text-ink-dim uppercase">
              files changed
            </span>
            <span className="font-mono text-[12px] font-bold text-ink">{files.length}</span>
          </div>
          {files.map((file) => (
            <button
              key={file.path}
              type="button"
              onClick={() => onShowDiffs(file.path)}
              className="flex w-full items-center gap-2 border-b border-line-soft py-1.5 text-left font-mono text-[11px] transition-colors last:border-b-0 hover:bg-surface2/50"
            >
              <span className="min-w-0 flex-1 truncate text-left text-ink-3 [direction:rtl]">
                {file.path}
              </span>
              <span className="text-green">+{file.additions}</span>
              <span className="text-red">−{file.deletions}</span>
            </button>
          ))}
        </div>
      )}
    </div>
  );
}
