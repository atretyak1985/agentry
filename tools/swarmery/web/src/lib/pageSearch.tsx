// Page-scoped header search: one search input lives in the app header and
// filters the CURRENT page's content (sessions by title, projects by name,
// system components by name, …). The query is shared through this context —
// the header owns the input, each page reads the query and filters its list.
// The query resets on navigation so a filter never leaks between sections.

import {
  createContext,
  useContext,
  useEffect,
  useMemo,
  useState,
  type ReactNode,
} from 'react';
import { useLocation } from 'react-router-dom';

interface PageSearchValue {
  /** Raw header-search text (trim/lowercase at the point of use). */
  query: string;
  setQuery: (q: string) => void;
}

const PageSearchContext = createContext<PageSearchValue>({
  query: '',
  setQuery: () => undefined,
});

export function PageSearchProvider({ children }: { children: ReactNode }): JSX.Element {
  const [query, setQuery] = useState('');
  const { pathname } = useLocation();
  // Clear the filter when moving to another section — a title filter on
  // /sessions must not carry over to /projects.
  useEffect(() => {
    setQuery('');
  }, [pathname]);
  const value = useMemo(() => ({ query, setQuery }), [query]);
  return <PageSearchContext.Provider value={value}>{children}</PageSearchContext.Provider>;
}

/** Header input binds to this (raw value + setter). */
export function usePageSearchControl(): PageSearchValue {
  return useContext(PageSearchContext);
}

/** Pages read the normalised (trimmed, lowercased) query to filter their list. */
export function usePageSearch(): string {
  return useContext(PageSearchContext).query.trim().toLowerCase();
}

/** Placeholder for the header input per route — null hides the input on pages
 * that have no searchable list (analytics, docs, detail views). */
export function pageSearchPlaceholder(pathname: string): string | null {
  if (pathname === '/') return 'filter sessions by title…';
  if (pathname === '/sessions') return 'filter sessions by title…';
  if (pathname === '/projects') return 'filter projects by name…';
  if (pathname === '/approvals') return 'filter approvals…';
  if (pathname === '/system') return 'filter by name…';
  return null;
}
