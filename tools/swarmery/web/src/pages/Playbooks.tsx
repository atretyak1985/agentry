// Playbooks page (fusion phase 13): a project's selectable execution recipes.
// The left list shows every playbook (name, source badge builtin/project, verify
// chip); selecting one opens a READ-ONLY stage-chain visualization — a
// horizontal row of stage boxes joined by arrows (plain flex + SVG, no graph
// lib) with each stage's rendered prompt preview. Built-ins are read-only;
// "Duplicate to project" copies the markdown into <project>/.claude/playbooks so
// its prompts become editable (the graduation rule) — after which a hint points
// at the on-disk path. Visual AUTHORING (editing the graph) is a follow-up.

import { useEffect, useMemo, useState } from 'react';
import type { Playbook } from '../api/types';
import { duplicatePlaybook, fetchPlaybooks } from '../api';
import { useProjectWorkspace } from '../workspace/ProjectContext';
import { VerifyChip } from '../workspace/PlaybookPicker';
import { Empty, ErrorBox, Loading } from '../components/ui';

export function Playbooks(): JSX.Element {
  const { project, projectId, loading: projLoading } = useProjectWorkspace();
  const [playbooks, setPlaybooks] = useState<Playbook[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [selected, setSelected] = useState<string | null>(null);

  const load = (): void => {
    if (projectId === null) return;
    setError(null);
    fetchPlaybooks(projectId)
      .then((ps) => {
        setPlaybooks(ps);
        const first = ps[0];
        setSelected((cur) => cur ?? (first !== undefined ? first.name : null));
      })
      .catch((e: unknown) => setError(e instanceof Error ? e.message : String(e)));
  };

  useEffect(() => {
    setPlaybooks(null);
    setSelected(null);
    load();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [projectId]);

  const active = useMemo(
    () => playbooks?.find((p) => p.name === selected) ?? null,
    [playbooks, selected],
  );

  if (projLoading) return <Loading label="workspace…" />;
  if (project === null) {
    return (
      <div className="px-4 py-8 desk:px-8">
        <Empty>unknown project — pick one from the switcher</Empty>
      </div>
    );
  }

  return (
    <div className="flex min-h-0 flex-1 flex-col px-4 py-5 desk:px-6">
      <header className="mb-4">
        <h1 className="text-[15px] font-semibold text-ink">Playbooks</h1>
        <p className="mt-0.5 text-[12px] text-ink-dim">
          Selectable execution recipes. A task picks one on the board; the dispatcher runs its stages
          sequentially in one worktree. Built-ins are read-only — duplicate one to edit its prompts.
        </p>
      </header>

      {error !== null && (
        <div className="mb-3">
          <ErrorBox message={error} onRetry={load} />
        </div>
      )}

      {playbooks === null ? (
        <Loading label="playbooks…" />
      ) : playbooks.length === 0 ? (
        <Empty>no playbooks</Empty>
      ) : (
        <div className="flex min-h-0 flex-1 flex-col gap-4 desk:flex-row">
          <PlaybookList playbooks={playbooks} selected={selected} onSelect={setSelected} />
          <div className="min-w-0 flex-1">
            {active !== null && projectId !== null ? (
              <PlaybookDetail playbook={active} projectId={projectId} onDuplicated={load} />
            ) : (
              <Empty>select a playbook</Empty>
            )}
          </div>
        </div>
      )}
    </div>
  );
}

function SourceBadge({ source }: { source: string }): JSX.Element {
  const project = source === 'project';
  return (
    <span
      className={`rounded-full border px-1.5 py-[1px] font-mono text-[9px] uppercase ${
        project ? 'border-brand/40 bg-brand/5 text-brand' : 'border-line text-ink-faint'
      }`}
    >
      {source}
    </span>
  );
}

function PlaybookList({
  playbooks,
  selected,
  onSelect,
}: {
  playbooks: Playbook[];
  selected: string | null;
  onSelect: (name: string) => void;
}): JSX.Element {
  return (
    <nav aria-label="playbooks" className="flex shrink-0 flex-col gap-1 desk:w-[228px]">
      {playbooks.map((p) => {
        const active = p.name === selected;
        return (
          <button
            key={p.name}
            type="button"
            onClick={() => onSelect(p.name)}
            aria-current={active}
            className={`flex flex-col gap-1 rounded-[10px] border px-3 py-2 text-left transition-colors ${
              active
                ? 'border-line-strong bg-surface2'
                : 'border-line bg-surface/40 hover:border-line-strong'
            }`}
          >
            <div className="flex items-center gap-1.5">
              <span className="min-w-0 flex-1 truncate font-mono text-[12px] text-ink">{p.name}</span>
              <SourceBadge source={p.source} />
            </div>
            <div className="flex items-center gap-1.5">
              <VerifyChip verify={p.verify} />
              <span className="font-mono text-[9px] text-ink-faint">
                {p.stages.length} stage{p.stages.length === 1 ? '' : 's'}
              </span>
            </div>
          </button>
        );
      })}
    </nav>
  );
}

function PlaybookDetail({
  playbook,
  projectId,
  onDuplicated,
}: {
  playbook: Playbook;
  projectId: number;
  onDuplicated: () => void;
}): JSX.Element {
  const [busy, setBusy] = useState(false);
  const [dupError, setDupError] = useState<string | null>(null);
  const [hint, setHint] = useState<string | null>(null);

  // Reset the transient duplicate feedback when a different playbook is opened.
  useEffect(() => {
    setDupError(null);
    setHint(null);
  }, [playbook.name]);

  const duplicate = (): void => {
    setBusy(true);
    setDupError(null);
    duplicatePlaybook(projectId, playbook.name)
      .then((res) => {
        setHint(res.hint);
        onDuplicated();
      })
      .catch((e: unknown) => setDupError(e instanceof Error ? e.message : String(e)))
      .finally(() => setBusy(false));
  };

  const isBuiltin = playbook.source === 'builtin';

  return (
    <section className="flex min-h-0 flex-col rounded-xl border border-line bg-surface/40 p-4">
      <div className="mb-1 flex flex-wrap items-center gap-2">
        <h2 className="font-mono text-[14px] text-ink">{playbook.name}</h2>
        <SourceBadge source={playbook.source} />
        <VerifyChip verify={playbook.verify} />
        {playbook.model !== '' && (
          <span className="rounded border border-line px-1 py-[1px] font-mono text-[9px] text-ink-dim">
            model {playbook.model}
          </span>
        )}
      </div>
      <p className="mb-3 text-[12px] text-ink-dim">{playbook.description}</p>

      <StageChain playbook={playbook} />

      {playbook.path !== '' && (
        <p className="mt-3 font-mono text-[10px] text-ink-faint">
          project file: <span className="text-ink-dim">{playbook.path}</span>
        </p>
      )}

      <div className="mt-4 border-t border-line pt-3">
        {isBuiltin ? (
          <div className="flex flex-wrap items-center gap-2">
            <button
              type="button"
              disabled={busy}
              onClick={duplicate}
              className="rounded-lg border border-brand/50 bg-brand/10 px-3 py-1.5 text-[12px] font-semibold text-brand transition-colors hover:bg-brand/20 disabled:cursor-not-allowed disabled:opacity-40"
            >
              {busy ? 'Duplicating…' : 'Duplicate to project'}
            </button>
            <span className="font-mono text-[10px] text-ink-faint">
              copies the markdown into the project so its prompts become editable
            </span>
          </div>
        ) : (
          <p className="font-mono text-[10px] text-ink-faint">
            this is a project playbook — edit its file directly; it overrides the built-in of the same name
          </p>
        )}
        {hint !== null && (
          <div className="mt-2 rounded-md border border-brand/30 bg-brand/5 px-2 py-1.5 font-mono text-[10.5px] text-brand">
            {hint}
          </div>
        )}
        {dupError !== null && (
          <div className="mt-2 font-mono text-[10.5px] text-red">{dupError}</div>
        )}
      </div>
    </section>
  );
}

/**
 * Read-only horizontal stage chain: stage boxes joined by arrows. Each box shows
 * the stage index + name and a monospace preview of the stage's prompt body.
 * Plain flex + inline arrow glyphs (no graph lib, per the phase spec).
 */
function StageChain({ playbook }: { playbook: Playbook }): JSX.Element {
  return (
    <div className="overflow-x-auto">
      <ol className="flex items-stretch gap-0" aria-label="stage chain">
        {playbook.stages.map((stage, i) => (
          <li key={i} className="flex items-stretch">
            <div className="flex w-[280px] shrink-0 flex-col rounded-lg border border-line bg-field p-2.5">
              <div className="mb-1.5 flex items-center gap-1.5">
                <span className="flex h-[18px] w-[18px] shrink-0 items-center justify-center rounded-full bg-brand/15 font-mono text-[10px] text-brand">
                  {i + 1}
                </span>
                <span className="min-w-0 flex-1 truncate font-mono text-[11.5px] text-ink">{stage.name}</span>
              </div>
              <pre className="max-h-[160px] overflow-y-auto whitespace-pre-wrap break-words rounded-md bg-bg/60 p-2 font-mono text-[10px] leading-relaxed text-ink-2">
                {stage.body}
              </pre>
            </div>
            {i < playbook.stages.length - 1 && (
              <div
                aria-hidden="true"
                className="flex w-8 shrink-0 items-center justify-center text-[16px] text-ink-faint"
              >
                →
              </div>
            )}
          </li>
        ))}
      </ol>
    </div>
  );
}
