// Shared primitives of the System screen (phase 4, Stage 1 read-only):
// scope/origin/lint badges, the scope+project filter row, and the list-fetch
// hook every tab uses. Visual language mirrors components/ui.tsx (hairline
// pill chips, mono micro-type); tooltips are native `title` attributes.

import { useEffect, useRef, useState } from 'react';
import type { LintSeverity, Project } from '../../api/types';
import type { SystemListFilters } from '../../api/system';
import { projectColor } from '../../lib/colors';

/* ----- badges ----- */

export function ScopeBadge({
  scope,
  projectSlug,
  projectName,
}: {
  scope: 'global' | 'project';
  projectSlug: string | null;
  projectName?: string | null | undefined;
}): JSX.Element {
  if (scope === 'project') {
    const label = projectName ?? projectSlug;
    return (
      <span
        className="rounded-full border border-blue/40 px-2 py-px font-mono text-[10px] whitespace-nowrap text-blue"
        title={projectSlug ?? undefined}
      >
        project{label !== null ? ` · ${label}` : ''}
      </span>
    );
  }
  return (
    <span className="rounded-full border border-line px-2 py-px font-mono text-[10px] whitespace-nowrap text-ink-dim">
      global
    </span>
  );
}

export function OriginBadge({
  origin,
  pluginName,
}: {
  origin: 'local' | 'plugin';
  pluginName: string | null;
}): JSX.Element {
  if (origin === 'plugin') {
    return (
      <span className="rounded-full border border-brand/40 px-2 py-px font-mono text-[10px] whitespace-nowrap text-brand">
        plugin{pluginName !== null ? ` · ${pluginName}` : ''}
      </span>
    );
  }
  return (
    <span className="rounded-full border border-line px-2 py-px font-mono text-[10px] whitespace-nowrap text-ink-dim">
      local
    </span>
  );
}

export const LINT_TONES: Record<LintSeverity, string> = {
  error: 'text-red',
  warn: 'text-amber',
  info: 'text-blue',
};

/** Max-severity lint marker of a list row; null (clean) renders nothing. */
export function LintDot({
  severity,
  message,
}: {
  severity: LintSeverity | null;
  message?: string;
}): JSX.Element | null {
  if (severity === null) return null;
  return (
    <span
      className={`shrink-0 font-mono text-[12px] leading-none ${LINT_TONES[severity]}`}
      title={message ?? `worst active lint finding: ${severity}`}
      aria-label={`lint ${severity}`}
    >
      {severity === 'info' ? '●' : '▲'}
    </span>
  );
}

/* ----- filter chips (Sessions FilterChip rhythm) ----- */

export function FilterChip({
  selected,
  onClick,
  children,
}: {
  selected: boolean;
  onClick: () => void;
  children: string;
}): JSX.Element {
  return (
    <button
      type="button"
      onClick={onClick}
      aria-pressed={selected}
      className={`shrink-0 rounded-full border px-2.5 py-[3px] font-mono text-[10.5px] whitespace-nowrap transition-colors ${
        selected ? 'border-ink-dim bg-surface2 text-ink' : 'border-line text-ink-dim hover:text-ink'
      }`}
    >
      {children}
    </button>
  );
}

/* ----- project dropdown (Sessions-style headless select) ----- */

const ALL_DOT = '#7c8da3';

function Dot({ color }: { color: string }): JSX.Element {
  return (
    <span className="h-1.5 w-1.5 shrink-0 rounded-full" style={{ background: color }} aria-hidden="true" />
  );
}

function DropdownOption({
  selected,
  dot,
  label,
  onSelect,
}: {
  selected: boolean;
  dot: string;
  label: string;
  onSelect: () => void;
}): JSX.Element {
  return (
    <button
      type="button"
      role="option"
      aria-selected={selected}
      onClick={onSelect}
      className={`flex w-full items-center gap-2 px-3 py-1.5 text-left font-mono text-[11px] transition-colors hover:bg-surface2 ${selected ? 'text-ink' : 'text-ink-dim'}`}
    >
      <Dot color={dot} />
      <span className="min-w-0 flex-1 truncate">{label}</span>
      {selected && <span aria-hidden="true">✓</span>}
    </button>
  );
}

export function ProjectDropdown({
  projects,
  value,
  onChange,
}: {
  projects: Project[];
  value: string | null;
  onChange: (slug: string | null) => void;
}): JSX.Element {
  const [open, setOpen] = useState(false);
  const rootRef = useRef<HTMLDivElement>(null);
  const buttonRef = useRef<HTMLButtonElement>(null);
  const menuRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!open) return undefined;
    const onPointerDown = (e: MouseEvent): void => {
      if (rootRef.current !== null && !rootRef.current.contains(e.target as Node)) setOpen(false);
    };
    const onKeyDown = (e: KeyboardEvent): void => {
      if (e.key === 'Escape') { setOpen(false); buttonRef.current?.focus(); }
    };
    document.addEventListener('mousedown', onPointerDown);
    document.addEventListener('keydown', onKeyDown);
    return () => {
      document.removeEventListener('mousedown', onPointerDown);
      document.removeEventListener('keydown', onKeyDown);
    };
  }, [open]);

  const focusOption = (delta: 1 | -1): void => {
    const opts = menuRef.current?.querySelectorAll<HTMLButtonElement>('[role="option"]');
    if (!opts?.length) return;
    const list = Array.from(opts);
    const idx = list.indexOf(document.activeElement as HTMLButtonElement);
    list[(idx + delta + list.length) % list.length]?.focus();
  };

  const select = (slug: string | null): void => {
    onChange(slug);
    setOpen(false);
    buttonRef.current?.focus();
  };

  const selected = value !== null ? (projects.find((p) => p.slug === value) ?? null) : null;
  const label = value === null ? 'all projects' : (selected?.name ?? selected?.slug ?? value);
  const dot = value === null ? ALL_DOT : projectColor(value);

  return (
    <div ref={rootRef} className="relative shrink-0">
      <button
        ref={buttonRef}
        type="button"
        aria-haspopup="listbox"
        aria-expanded={open}
        aria-label="filter by project"
        onClick={() => setOpen((v) => !v)}
        onKeyDown={(e) => { if (e.key === 'ArrowDown' && open) { e.preventDefault(); focusOption(1); } }}
        className="flex max-w-[200px] items-center gap-1.5 rounded-full border border-line px-2.5 py-[3px] font-mono text-[10.5px] whitespace-nowrap text-ink-dim transition-colors hover:text-ink aria-expanded:border-ink-dim aria-expanded:bg-surface2 aria-expanded:text-ink"
      >
        <Dot color={dot} />
        <span className="truncate">{label}</span>
        <span aria-hidden="true" className="text-[8px]">▾</span>
      </button>
      {open && (
        <div
          ref={menuRef}
          role="listbox"
          aria-label="project"
          onKeyDown={(e) => {
            if (e.key === 'ArrowDown' || e.key === 'ArrowUp') { e.preventDefault(); focusOption(e.key === 'ArrowDown' ? 1 : -1); }
          }}
          className="absolute top-full left-0 z-20 mt-1 max-h-60 min-w-[180px] overflow-y-auto rounded-lg border border-line bg-surface py-1 shadow-xl shadow-black/40"
        >
          <DropdownOption selected={value === null} dot={ALL_DOT} label="all projects" onSelect={() => select(null)} />
          {projects.map((p) => (
            <DropdownOption
              key={p.id}
              selected={value === p.slug}
              dot={projectColor(p.slug)}
              label={p.name ?? p.slug}
              onSelect={() => select(p.slug)}
            />
          ))}
        </div>
      )}
    </div>
  );
}

/** Search input + project dropdown + scope chips — the top filter bar of every
 * System tab. Search is client-side; scope/project are pushed to the API. */
export function FiltersRow({
  scope,
  project,
  projects,
  search,
  onSearch,
  onScope,
  onProject,
}: {
  scope: 'global' | 'project' | null;
  project: string | null;
  projects: Project[];
  search: string;
  onSearch: (s: string) => void;
  onScope: (scope: 'global' | 'project' | null) => void;
  onProject: (slug: string | null) => void;
}): JSX.Element {
  return (
    <div className="mt-3 flex flex-wrap items-center gap-x-2 gap-y-2">
      <div className="relative w-full desk:w-[240px]">
        <input
          type="text"
          value={search}
          onChange={(e) => onSearch(e.target.value)}
          placeholder="filter by name…"
          aria-label="filter by name"
          className="w-full rounded-lg border border-line bg-surface px-3 py-[6px] pr-8 font-mono text-[12px] text-ink transition-colors outline-none placeholder:text-ink-dim focus:border-ink-dim"
        />
        {search !== '' && (
          <button
            type="button"
            onClick={() => onSearch('')}
            aria-label="clear search"
            className="absolute top-1/2 right-2 -translate-y-1/2 font-mono text-[13px] leading-none text-ink-dim transition-colors hover:text-ink"
          >
            ×
          </button>
        )}
      </div>
      <ProjectDropdown projects={projects} value={project} onChange={onProject} />
      <span className="mx-1 w-px shrink-0 self-stretch bg-line" aria-hidden="true" />
      <FilterChip selected={scope === null} onClick={() => onScope(null)}>all scopes</FilterChip>
      <FilterChip selected={scope === 'global'} onClick={() => onScope('global')}>global</FilterChip>
      <FilterChip selected={scope === 'project'} onClick={() => onScope('project')}>project</FilterChip>
    </div>
  );
}

/* ----- list-fetch hook ----- */

interface SystemListState<T> {
  rows: T[] | null;
  error: string | null;
  /** Project slugs seen in the last UNfiltered response (chip options). */
  projectOptions: string[];
  retry: () => void;
}

/**
 * Fetches one /api/system list whenever filters or refreshKey change
 * (refreshKey bumps on WS system_item_updated → refetch of the open tab).
 */
export function useSystemList<T extends { projectSlug: string | null }>(
  fetcher: (filters: SystemListFilters) => Promise<T[]>,
  scope: 'global' | 'project' | null,
  project: string | null,
  refreshKey: number,
): SystemListState<T> {
  const [rows, setRows] = useState<T[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [projectOptions, setProjectOptions] = useState<string[]>([]);
  const [attempt, setAttempt] = useState(0);

  useEffect(() => {
    let cancelled = false;
    const filters: SystemListFilters = {};
    if (scope !== null) filters.scope = scope;
    if (project !== null) filters.project = project;
    fetcher(filters)
      .then((list) => {
        if (cancelled) return;
        setRows(list);
        setError(null);
        if (project === null) {
          const slugs = [...new Set(list.map((r) => r.projectSlug).filter((s) => s !== null))];
          setProjectOptions(slugs.sort());
        }
      })
      .catch((e: unknown) => {
        if (!cancelled) setError(String(e));
      });
    return () => {
      cancelled = true;
    };
  }, [fetcher, scope, project, refreshKey, attempt]);

  return { rows, error, projectOptions, retry: () => setAttempt((a) => a + 1) };
}
