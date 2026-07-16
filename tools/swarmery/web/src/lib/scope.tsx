// Global project scope (ScopeContext): one selected project slug (or null =
// all projects) shared by every page as its DEFAULT project filter — the
// GitHub-org-switcher pattern. The selection persists in localStorage and is
// reflected as ?scope=<slug> on the URL when it changes; on first load a URL
// param wins over the stored value so deep links work. NOTE: /system's
// component-scope query param was renamed to ?level= so the global ?scope=
// owns the name everywhere.

import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
  type ReactNode,
} from 'react';
import { useSearchParams } from 'react-router-dom';
import type { Project } from '../api/types';
import { fetchProjects } from '../api';

const STORAGE_KEY = 'swarmery.scope';

interface ScopeValue {
  /** Selected project slug, or null = all projects. */
  scope: string | null;
  setScope: (slug: string | null) => void;
  /** Non-archived projects, fetched once here and shared by every consumer
   * (header switcher, command palette, …) — display names live on these. */
  projects: Project[];
  /** Clean display name for the current scope (never the raw path slug). */
  scopeName: string | null;
}

const ScopeContext = createContext<ScopeValue>({
  scope: null,
  setScope: () => undefined,
  projects: [],
  scopeName: null,
});

function storedScope(): string | null {
  try {
    const v = window.localStorage.getItem(STORAGE_KEY);
    return v !== null && v !== '' ? v : null;
  } catch {
    return null; // storage disabled (private mode) → session-only scope
  }
}

export function ScopeProvider({ children }: { children: ReactNode }): JSX.Element {
  const [searchParams, setSearchParams] = useSearchParams();
  // URL wins over localStorage on first load (?scope= deep links).
  const [scope, setScopeState] = useState<string | null>(
    () => searchParams.get('scope') ?? storedScope(),
  );

  const setScope = useCallback(
    (slug: string | null): void => {
      setScopeState(slug);
      try {
        if (slug === null) window.localStorage.removeItem(STORAGE_KEY);
        else window.localStorage.setItem(STORAGE_KEY, slug);
      } catch {
        // storage disabled — the in-memory scope still applies this session
      }
      setSearchParams(
        (prev) => {
          const next = new URLSearchParams(prev);
          if (slug === null) next.delete('scope');
          else next.set('scope', slug);
          return next;
        },
        { replace: true },
      );
    },
    [setSearchParams],
  );

  // Back/forward navigation that changes ?scope= re-syncs the context.
  const urlScope = searchParams.get('scope');
  useEffect(() => {
    if (urlScope !== null && urlScope !== scope) setScopeState(urlScope);
  }, [urlScope, scope]);

  const [projects, setProjects] = useState<Project[]>([]);
  useEffect(() => {
    fetchProjects()
      .then(setProjects)
      .catch(() => setProjects([])); // consumers degrade to slug labels
  }, []);

  const value = useMemo(() => {
    const selected = scope !== null ? (projects.find((p) => p.slug === scope) ?? null) : null;
    const scopeName = scope === null ? null : (selected?.name ?? scope);
    return { scope, setScope, projects, scopeName };
  }, [scope, setScope, projects]);
  return <ScopeContext.Provider value={value}>{children}</ScopeContext.Provider>;
}

export function useScope(): ScopeValue {
  return useContext(ScopeContext);
}
