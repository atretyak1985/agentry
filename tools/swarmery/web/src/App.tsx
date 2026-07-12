// App shell (Redesign frame): a full-width top header bar — SW◆RMERY wordmark
// at the far left (over the sidebar column), "● control plane · :<port>" at
// the right, border-b spanning the full width — with the sidebar starting
// BELOW the header and <main> as the app's scroll container (so the session
// detail can pin its header and scroll only the tab panel). Mobile keeps the
// bottom nav. The Docs nav item appears only when /api/docs has entries; the
// desktop Sessions item carries a today-count badge (/api/stats/overview) and
// the sidebar bottom shows the daemon health line.

import { useEffect, useState } from 'react';
import { NavLink, Outlet } from 'react-router-dom';
import { fetchDocs, fetchStatsOverview, MOCK } from './api';
import { isoDay } from './lib/format';
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

/** The dashboard's own port for the header line (":7777" default). */
function portLabel(): string {
  return window.location.port !== '' ? window.location.port : '7777';
}

export function App(): JSX.Element {
  const [hasDocs, setHasDocs] = useState(false);
  const [sessionsToday, setSessionsToday] = useState<number | null>(null);

  useEffect(() => {
    fetchDocs()
      .then((docs) => setHasDocs(docs.length > 0))
      .catch(() => setHasDocs(false)); // empty/unreachable → hide the Docs item
  }, []);

  useEffect(() => {
    // Sessions nav badge: one-shot fetch of today's overview so the count
    // works on every screen (hidden when unavailable).
    fetchStatsOverview(isoDay())
      .then((o) => setSessionsToday(o.sessions))
      .catch(() => setSessionsToday(null));
  }, []);

  const items = hasDocs ? [...BASE_NAV, DOCS_NAV] : BASE_NAV;

  return (
    <div className="flex h-dvh flex-col">
      <header className="z-20 flex h-12 shrink-0 items-center gap-2.5 border-b border-line bg-bg px-4 desk:px-5">
        <span className="font-display text-[17px] leading-none font-bold tracking-[0.06em]">
          SW<em className="text-brand not-italic">◆</em>RMERY
        </span>
        <span className="ml-auto flex items-center gap-1.5 font-mono text-[11px] text-ink-dim">
          {MOCK ? (
            <>
              <span className="inline-block h-[7px] w-[7px] rounded-full bg-amber" />
              {`mock data · :${portLabel()}`}
            </>
          ) : (
            <>
              <span className="inline-block h-[7px] w-[7px] animate-pulse-dot rounded-full bg-green" />
              {`control plane · :${portLabel()}`}
            </>
          )}
        </span>
      </header>

      <div className="flex min-h-0 flex-1">
        <nav className="fixed inset-x-0 bottom-0 z-20 flex justify-around border-t border-line bg-bg/95 px-1 pt-2 pb-[calc(8px+env(safe-area-inset-bottom))] backdrop-blur-md desk:static desk:inset-auto desk:z-auto desk:w-[210px] desk:shrink-0 desk:flex-col desk:justify-start desk:gap-1 desk:overflow-y-auto desk:border-t-0 desk:border-r desk:bg-bg desk:px-3 desk:py-4 desk:backdrop-blur-none">
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
              {item.to === '/sessions' && sessionsToday !== null && sessionsToday > 0 && (
                <span className="ml-auto hidden min-w-[18px] rounded-full bg-surface2 px-1.5 py-px text-center font-mono text-[10px] leading-[14px] text-ink-dim desk:block">
                  {sessionsToday}
                </span>
              )}
            </NavLink>
          ))}
          <span className="hidden desk:contents">
            <HealthFooter />
          </span>
        </nav>

        <main className="min-w-0 flex-1 overflow-y-auto [-webkit-overflow-scrolling:touch]">
          <div className="mx-auto h-full max-w-[1360px] p-4 pb-[88px] desk:px-7 desk:py-6">
            <Outlet />
          </div>
        </main>
      </div>
    </div>
  );
}
