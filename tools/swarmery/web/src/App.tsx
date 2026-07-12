// App shell: sticky app bar, bottom nav (mobile) / sidebar (≥900px), routed
// screens in <Outlet>. Redesign dark editorial language: navy surfaces,
// slate hairlines, Space Grotesk wordmark with the amber diamond. The Docs
// nav item appears only when /api/docs has entries; the sidebar bottom shows
// the daemon health line.

import { useEffect, useState } from 'react';
import { NavLink, Outlet } from 'react-router-dom';
import { fetchDocs, MOCK } from './api';
import { HealthFooter } from './components/HealthFooter';

interface NavItem {
  to: string;
  icon: string;
  label: string;
}

const BASE_NAV: NavItem[] = [
  { to: '/', icon: '◉', label: 'Overview' },
  { to: '/sessions', icon: '☰', label: 'Sessions' },
];

const DOCS_NAV: NavItem = { to: '/docs', icon: '❐', label: 'Docs' };

export function App(): JSX.Element {
  const [hasDocs, setHasDocs] = useState(false);

  useEffect(() => {
    fetchDocs()
      .then((docs) => setHasDocs(docs.length > 0))
      .catch(() => setHasDocs(false)); // empty/unreachable → hide the Docs item
  }, []);

  const items = hasDocs ? [...BASE_NAV, DOCS_NAV] : BASE_NAV;

  return (
    <div className="min-h-dvh pb-[76px] desk:pb-0 desk:pl-[210px]">
      <header className="sticky top-0 z-20 flex items-center gap-2.5 border-b border-line bg-bg/90 px-4 py-3 backdrop-blur-md">
        <span className="font-display text-[17px] leading-none font-bold tracking-[0.06em] desk:hidden">
          SW<em className="text-brand not-italic">◆</em>RMERY
        </span>
        <span className="ml-auto flex items-center gap-1.5 font-mono text-[11px] text-ink-dim">
          {MOCK ? (
            <>
              <span className="inline-block h-[7px] w-[7px] rounded-full bg-amber" />
              mock data
            </>
          ) : (
            <>
              <span className="inline-block h-[7px] w-[7px] animate-pulse-dot rounded-full bg-green" />
              control plane
            </>
          )}
        </span>
      </header>

      <nav className="fixed inset-x-0 bottom-0 z-20 flex justify-around border-t border-line bg-bg/95 px-1 pt-2 pb-[calc(8px+env(safe-area-inset-bottom))] backdrop-blur-md desk:top-0 desk:right-auto desk:bottom-0 desk:left-0 desk:w-[210px] desk:flex-col desk:justify-start desk:gap-1 desk:border-t-0 desk:border-r desk:px-3 desk:py-4">
        <span className="hidden desk:block px-3 pt-1 pb-5 font-display text-[17px] leading-none font-bold tracking-[0.06em] text-ink">
          SW<em className="text-brand not-italic">◆</em>RMERY
        </span>
        {items.map((item) => (
          <NavLink
            key={item.to}
            to={item.to}
            end={item.to === '/'}
            className={({ isActive }) =>
              `flex flex-col items-center gap-[3px] rounded-lg px-2.5 py-1 text-[10.5px] transition-colors desk:w-full desk:flex-row desk:justify-start desk:gap-2.5 desk:px-3 desk:py-2 desk:text-[13px] ${
                isActive ? 'font-medium text-brand desk:bg-surface2' : 'text-ink-dim hover:text-ink'
              }`
            }
          >
            <span className="text-[17px] leading-none" aria-hidden="true">
              {item.icon}
            </span>
            {item.label}
          </NavLink>
        ))}
        <span className="hidden desk:contents">
          <HealthFooter />
        </span>
      </nav>

      <main className="mx-auto max-w-[1360px] p-4 desk:px-7 desk:py-6">
        <Outlet />
      </main>
    </div>
  );
}
