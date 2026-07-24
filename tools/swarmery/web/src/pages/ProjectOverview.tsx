// Project overview (/p/:slug index — fusion phase 4): the rehomed body of the
// old ProjectDetail (header + stats + local component inventory + recent
// sessions), scoped by the workspace project instead of a :id route param. The
// plugin toggles + detach controls move to ProjectSettings (/p/:slug/settings);
// this page is the read-first landing tab. Telemetry-only projects hide the
// component sections, same as before.

import { useCallback, useEffect, useState } from 'react';
import { Link } from 'react-router-dom';
import type { ProjectComponent, ProjectDetail as ProjectDetailData } from '../api/types';
import { fetchProject } from '../api';
import { useProjectWorkspace } from '../workspace/ProjectContext';
import { fmtAgo, fmtCost, fmtDateTime, fmtTokens } from '../lib/format';
import { ProjectName } from '../components/ProjectName';
import { PluginBadge, ProjectActions } from '../components/ProjectActions';
import { Card, Empty, ErrorBox, Loading, SectionTitle } from '../components/ui';

function StatTile({ label, value }: { label: string; value: string }): JSX.Element {
  return (
    <div className="rounded-lg border border-line bg-surface px-3 py-2">
      <div className="font-mono text-[10px] tracking-[0.12em] text-ink-faint uppercase">{label}</div>
      <div className="mt-0.5 font-mono text-[13px] text-ink">{value}</div>
    </div>
  );
}

function ComponentList({ title, items }: { title: string; items: ProjectComponent[] }): JSX.Element {
  return (
    <div>
      <div className="font-mono text-[10.5px] tracking-[0.1em] text-ink-dim uppercase">
        {title} · {items.length}
      </div>
      {items.length === 0 ? (
        <div className="mt-1.5 font-mono text-[11px] text-ink-faint">none</div>
      ) : (
        <div className="mt-1.5 flex flex-wrap gap-1.5">
          {items.map((c) => (
            <span
              key={c.name}
              title={`source: ${c.source}`}
              className="rounded-full border border-line px-2 py-0.5 font-mono text-[10.5px] text-ink-2"
            >
              {c.name}
            </span>
          ))}
        </div>
      )}
    </div>
  );
}

export function ProjectOverview(): JSX.Element {
  const { projectId, loading: projLoading } = useProjectWorkspace();
  const [data, setData] = useState<ProjectDetailData | null>(null);
  const [error, setError] = useState<string | null>(null);

  const load = useCallback((): void => {
    if (projectId === null) return;
    fetchProject(projectId)
      .then((d) => {
        setData(d);
        setError(null);
      })
      .catch((e: unknown) => setError(e instanceof Error ? e.message : String(e)));
  }, [projectId]);

  useEffect(() => {
    setData(null);
    load();
  }, [load]);

  const wrap = (inner: JSX.Element): JSX.Element => (
    <div className="px-4 pt-5 pb-10 desk:px-8 desk:pt-7">{inner}</div>
  );

  if (projLoading && projectId === null) return wrap(<Loading label="workspace…" />);
  if (projectId === null) return wrap(<Empty>unknown project</Empty>);
  if (error !== null) return wrap(<ErrorBox message={error} onRetry={load} />);
  if (data === null) return wrap(<Loading label="project…" />);

  const { project, components, stats } = data;
  const managed = project.plugin?.managed ?? false;

  return wrap(
    <>
      <div className="flex flex-wrap items-center gap-x-2.5 gap-y-1.5">
        <ProjectName
          name={project.name}
          slug={project.slug}
          className="font-display text-[22px] font-medium tracking-[-0.01em] desk:text-[26px]"
        />
        <PluginBadge project={project} />
        {(project.plugin?.packs ?? []).map((pack) => (
          <span
            key={pack}
            className="rounded-full border border-brand/40 bg-brand/10 px-2 py-0.5 font-mono text-[10px] whitespace-nowrap text-brand"
          >
            {pack}
          </span>
        ))}
        {project.archived && (
          <span className="rounded-full border border-line px-2 py-0.5 font-mono text-[10px] whitespace-nowrap text-ink-faint">
            archived
          </span>
        )}
        <div className="ml-auto">
          <ProjectActions project={project} onChanged={load} />
        </div>
      </div>
      <div className="mt-1.5 font-mono text-[11px] text-ink-faint" title={project.path}>
        {project.path}
      </div>

      <SectionTitle>stats</SectionTitle>
      <div className="grid grid-cols-2 gap-2 desk:grid-cols-4">
        <StatTile label="sessions" value={String(stats.sessions)} />
        <StatTile label="tokens" value={stats.tokens !== null ? fmtTokens(stats.tokens) : '—'} />
        <StatTile label="cost" value={fmtCost(stats.costUsd)} />
        <StatTile
          label="last activity"
          value={stats.lastActivity !== null ? fmtAgo(stats.lastActivity) : '—'}
        />
      </div>

      {managed ? (
        <>
          <SectionTitle>components (local)</SectionTitle>
          <div className="space-y-3.5">
            <ComponentList title="agents" items={components.agents} />
            <ComponentList title="skills" items={components.skills} />
            <ComponentList title="commands" items={components.commands} />
            <ComponentList title="hooks" items={components.hooks} />
          </div>
          <div className="mt-3 font-mono text-[11px] text-ink-faint">
            manage plugins + detach in{' '}
            <Link to={`/p/${project.slug}/settings`} className="text-ink-dim underline hover:text-ink">
              Settings →
            </Link>
          </div>
        </>
      ) : (
        <>
          <SectionTitle>components</SectionTitle>
          <div className="rounded-xl border border-dashed border-line px-3.5 py-4 font-mono text-[11.5px] text-ink-dim">
            {project.plugin === null
              ? 'telemetry-only — no .claude/settings.json, the swarmery plugin is not installed here'
              : 'the swarmery plugin is not enabled for this project'}
          </div>
        </>
      )}

      <SectionTitle>recent sessions</SectionTitle>
      {stats.recentSessions.length === 0 ? (
        <div className="font-mono text-[11.5px] text-ink-faint">no sessions yet</div>
      ) : (
        <Card>
          <div className="divide-y divide-line-soft">
            {stats.recentSessions.map((s) => (
              <Link
                key={s.id}
                to={`/sessions/${String(s.id)}`}
                className="flex flex-wrap items-center gap-x-3 gap-y-0.5 py-1.5 font-mono text-[11px] transition-colors first:pt-0 last:pb-0 hover:text-ink"
              >
                <span className="min-w-0 flex-1 truncate text-ink-2">{s.title ?? s.sessionUuid}</span>
                <span className="text-ink-faint">{s.status}</span>
                {s.model !== null && <span className="text-ink-faint">{s.model}</span>}
                <span className="text-ink-faint">{fmtCost(s.costUsd)}</span>
                <span className="text-ink-faint">{fmtDateTime(s.startedAt)}</span>
              </Link>
            ))}
          </div>
        </Card>
      )}
    </>,
  );
}
