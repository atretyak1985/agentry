// Projects list (sidebar "Projects"): every project the daemon knows about,
// flagged managed (swarmery plugin enabled in its .claude/settings.json) vs
// telemetry-only, with lifetime session/token/cost totals. Each row links to
// the project detail (/projects/:id); row actions (archive / restore / detach)
// live in the shared ProjectActions control.

import { useCallback, useEffect, useState } from 'react';
import { Link } from 'react-router-dom';
import type { Project } from '../api/types';
import { fetchProjects } from '../api';
import { fmtAgo, fmtCost, fmtTokens } from '../lib/format';
import { ProjectName } from '../components/ProjectName';
import { PluginBadge, ProjectActions } from '../components/ProjectActions';
import { Card, Empty, ErrorBox, Loading } from '../components/ui';

function Metric({ label, value }: { label: string; value: string }): JSX.Element {
  return (
    <span className="whitespace-nowrap">
      <span className="text-ink-2">{value}</span>
      <span className="text-ink-faint"> {label}</span>
    </span>
  );
}

function ProjectRow({ project, onChanged }: { project: Project; onChanged: () => void }): JSX.Element {
  const packs = project.plugin?.packs ?? [];
  return (
    <Card>
      <div className="flex flex-wrap items-center gap-x-2.5 gap-y-1.5">
        <Link to={`/projects/${String(project.id)}`} className="group flex items-center gap-2">
          <ProjectName
            name={project.name}
            slug={project.slug}
            className="font-display text-[14px] font-semibold group-hover:underline"
          />
        </Link>
        <PluginBadge project={project} />
        {packs.map((pack) => (
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
          <ProjectActions project={project} onChanged={onChanged} />
        </div>
      </div>

      <div className="mt-1.5 flex flex-wrap items-center gap-x-3 gap-y-1 font-mono text-[11px] text-ink-dim">
        <Metric label="sessions" value={String(project.sessions)} />
        <Metric label="tokens" value={project.tokens !== null ? fmtTokens(project.tokens) : '—'} />
        <span className="whitespace-nowrap text-ink-2">{fmtCost(project.costUsd)}</span>
        {project.lastActivity !== null && (
          <span className="whitespace-nowrap text-ink-faint">{fmtAgo(project.lastActivity)}</span>
        )}
      </div>
      <div className="mt-1 truncate font-mono text-[10.5px] text-ink-faint" title={project.path}>
        {project.path}
      </div>
    </Card>
  );
}

export function Projects(): JSX.Element {
  const [projects, setProjects] = useState<Project[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [showArchived, setShowArchived] = useState(false);

  const load = useCallback((): void => {
    fetchProjects(showArchived)
      .then((list) => {
        setProjects(list);
        setError(null);
      })
      .catch((e: unknown) => setError(String(e)));
  }, [showArchived]);

  useEffect(() => {
    setProjects(null);
    load();
  }, [load]);

  const managed = (projects ?? []).filter((p) => p.plugin?.managed).length;

  return (
    <div className="px-4 pt-6 pb-20 desk:px-10 desk:pt-[34px] desk:pb-28">
      <div className="flex flex-wrap items-baseline justify-between gap-2">
        <h1 className="font-display text-[26px] font-medium tracking-[-0.01em] desk:text-[30px]">
          Projects
        </h1>
        <label className="flex cursor-pointer items-center gap-1.5 font-mono text-[11px] text-ink-dim">
          <input
            type="checkbox"
            checked={showArchived}
            onChange={(e) => setShowArchived(e.target.checked)}
            className="accent-brand"
          />
          show archived
        </label>
      </div>
      <div className="mt-1.5 font-mono text-[11px] text-ink-dim">
        {projects !== null
          ? `${String(projects.length)} project${projects.length === 1 ? '' : 's'} · ${String(managed)} managed`
          : ' '}
      </div>

      {error !== null && <ErrorBox message={error} onRetry={load} />}
      {projects === null && error === null && <Loading label="projects…" />}
      {projects !== null && projects.length === 0 && (
        <Empty>
          no projects yet — run{' '}
          <span className="font-mono text-ink">swarmery ingest &lt;file.jsonl&gt;</span> or onboard
          one from the command deck
        </Empty>
      )}

      <div className="mt-5">
        {(projects ?? []).map((p) => (
          <ProjectRow key={p.id} project={p} onChanged={load} />
        ))}
      </div>
    </div>
  );
}
