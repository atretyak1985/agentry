// "Run now" — the Agent Hub's dispatch shortcut. Per the phase spec it lands on
// a project board with QuickEntry prefilled `@<agent>: ` (Board reads ?compose=).
// When a project scope is active (workspace mount, or the fleet scope switcher
// is set) it navigates straight there; otherwise it opens a small project
// picker first (the spec's "ask to pick a project" step), reusing the shared
// projects list from the scope store.

import { useEffect, useRef, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { useScope } from '../../lib/scope';

/** Build the board deep-link that prefills QuickEntry with `@<agent>: `. */
function composeHref(slug: string, agentName: string): string {
  const prompt = `@${agentName}: `;
  return `/p/${encodeURIComponent(slug)}/board?compose=${encodeURIComponent(prompt)}`;
}

export function RunNowButton({
  agentName,
  scopeSlug,
}: {
  agentName: string;
  /** Active project scope (workspace slug or fleet scope); null = fleet, unscoped. */
  scopeSlug: string | null;
}): JSX.Element {
  const navigate = useNavigate();
  const { projects } = useScope();
  const [picking, setPicking] = useState(false);
  const wrapRef = useRef<HTMLDivElement>(null);

  // Dismiss the picker on an outside click / Escape.
  useEffect(() => {
    if (!picking) return;
    const onDown = (e: MouseEvent): void => {
      if (wrapRef.current !== null && !wrapRef.current.contains(e.target as Node)) setPicking(false);
    };
    const onKey = (e: KeyboardEvent): void => {
      if (e.key === 'Escape') setPicking(false);
    };
    window.addEventListener('mousedown', onDown);
    window.addEventListener('keydown', onKey);
    return () => {
      window.removeEventListener('mousedown', onDown);
      window.removeEventListener('keydown', onKey);
    };
  }, [picking]);

  const go = (slug: string): void => {
    setPicking(false);
    navigate(composeHref(slug, agentName));
  };

  const onClick = (): void => {
    if (scopeSlug !== null && scopeSlug !== '') {
      go(scopeSlug);
      return;
    }
    // Fleet, unscoped: a single project short-circuits the picker.
    if (projects.length === 1 && projects[0] !== undefined) {
      go(projects[0].slug);
      return;
    }
    setPicking((v) => !v);
  };

  return (
    <div ref={wrapRef} className="relative">
      <button
        type="button"
        onClick={onClick}
        className="rounded-lg border border-brand/40 bg-brand/10 px-3 py-1.5 text-[12px] font-semibold text-brand transition-colors hover:bg-brand/20"
        title={`prefill a new board task with @${agentName}:`}
      >
        ▸ Run now
      </button>
      {picking && (
        <div className="absolute right-0 z-30 mt-1 max-h-[280px] w-[220px] overflow-y-auto rounded-lg border border-line-strong bg-surface py-1 shadow-lg">
          <div className="px-3 py-1.5 font-mono text-[10px] tracking-[0.1em] text-ink-faint uppercase">
            pick a project
          </div>
          {projects.length === 0 && (
            <div className="px-3 py-2 font-mono text-[11px] text-ink-dim">no projects</div>
          )}
          {projects.map((p) => (
            <button
              key={p.slug}
              type="button"
              onClick={() => go(p.slug)}
              className="block w-full px-3 py-1.5 text-left text-[12.5px] text-ink-dim transition-colors hover:bg-surface2 hover:text-ink"
            >
              {p.name ?? p.slug}
            </button>
          ))}
        </div>
      )}
    </div>
  );
}
