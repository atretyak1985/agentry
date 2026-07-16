// App shell: a full-width top header (SW◆RMERY wordmark at left, a mono
// breadcrumb for the current screen, and a live control-plane/daemon status at
// right) with a bottom border. Below it, a static labelled sidebar (248px,
// desktop only) carries the glyph nav items and a live daemon-health footer;
// <main> owns the scroll. Mobile drops the sidebar for a fixed bottom nav.
//
// The Docs nav item appears only when /api/docs has entries; the Sessions item
// carries a today-count badge (/api/stats/overview); the Approvals item carries
// a LIVE amber pending-count badge (REST resync + WS permission_requested/
// permission_resolved over the shared connection).

import { useCallback, useEffect, useState } from 'react';
import { NavLink, Outlet, useLocation } from 'react-router-dom';
import type { WSMessage } from './api/types';
import { fetchApprovals, fetchDocs, fetchStatsOverview, MOCK } from './api';
import { fetchSystemSummary } from './api/system';
import { CommandPalette } from './components/CommandPalette';
import { NewProjectButton } from './components/NewProjectButton';
import { NotifySettings } from './components/NotifySettings';
import { ProjectDropdown } from './components/ProjectDropdown';
import { isoDay } from './lib/format';
import { useHealth, shortVersion } from './lib/health';
import { loadPrefs, useBrowserNotifications, type NotifyPrefs } from './lib/notifications';
import { ScopeProvider, useScope } from './lib/scope';
import { useLiveUpdates } from './lib/ws';

interface NavItem {
  to: string;
  glyph: string;
  label: string;
  /** Count badge (approvals pending, sessions today). */
  badge?: string;
  /** Amber attention styling on the badge (pending approvals). */
  alert?: boolean;
}

const DOCS_NAV: NavItem = { to: '/docs', glyph: '❐', label: 'Docs' };

/** Current route → mono breadcrumb (Canvas crumbMap). */
function crumbFor(pathname: string): string {
  if (pathname === '/') return 'control plane';
  if (pathname.startsWith('/sessions/')) return 'session';
  if (pathname.startsWith('/sessions')) return 'sessions';
  if (pathname.startsWith('/projects/')) return 'project';
  if (pathname.startsWith('/projects')) return 'projects';
  if (pathname.startsWith('/approvals')) return 'approvals';
  if (pathname.startsWith('/system')) return 'system';
  if (pathname.startsWith('/docs')) return 'docs';
  return '';
}

/** Global project scope switcher (header) — GitHub-org-switcher pattern.
 * Projects come from the ScopeProvider's shared fetch. */
function ScopeSwitcher(): JSX.Element {
  const { scope, setScope, projects } = useScope();
  return (
    <ProjectDropdown
      projects={projects}
      value={scope}
      onChange={setScope}
      allLabel="All projects"
      groupByTag
    />
  );
}

export function App(): JSX.Element {
  return (
    <ScopeProvider>
      <AppShell />
    </ScopeProvider>
  );
}

function AppShell(): JSX.Element {
  const [hasDocs, setHasDocs] = useState(false);
  const [sessionsToday, setSessionsToday] = useState<number | null>(null);
  // Pending approvals as a SET of ids: WS +/- stays idempotent when the same
  // permission_resolved arrives twice (own action + fan-out) or after resync.
  const [pendingIds, setPendingIds] = useState<ReadonlySet<number>>(new Set());
  const [paletteOpen, setPaletteOpen] = useState(false);
  // Browser notifications (control-plane v2): prefs from localStorage, the
  // hook rides the same shared WS connection as the badge below.
  const [notifyPrefs, setNotifyPrefs] = useState<NotifyPrefs>(loadPrefs);
  useBrowserNotifications(notifyPrefs);
  const { health, unreachable } = useHealth();
  const { pathname } = useLocation();
  const crumb = crumbFor(pathname);
  // Filter pages teleport their own controls into the header (#header-filters
  // slot via HeaderFilters portal) instead of the centered search pill.
  const filterPage = pathname === '/sessions' || pathname.startsWith('/system');

  useEffect(() => {
    fetchDocs()
      .then((docs) => setHasDocs(docs.length > 0))
      .catch(() => setHasDocs(false)); // empty/unreachable → hide the Docs item
  }, []);

  // Global Cmd+K / Ctrl+K → command palette. Window-level so it works from
  // any focused element; preventDefault stops the browser's own search-bar
  // focus shortcut.
  useEffect(() => {
    const onKeyDown = (e: KeyboardEvent): void => {
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'k') {
        e.preventDefault();
        setPaletteOpen((prev) => !prev);
      }
    };
    window.addEventListener('keydown', onKeyDown);
    return () => window.removeEventListener('keydown', onKeyDown);
  }, []);

  useEffect(() => {
    // Sessions nav badge: one-shot fetch of today's overview so the count
    // works on every screen (hidden when unavailable).
    fetchStatsOverview(isoDay())
      .then((o) => setSessionsToday(o.sessions))
      .catch(() => setSessionsToday(null));
  }, []);

  // System nav badge: promotion + stale-override insight count, fetched on
  // mount and refetched on WS system_item_updated (hidden when the summary is
  // unavailable) — pattern: sessions badge + approvals resync.
  const [insightCount, setInsightCount] = useState<number | null>(null);
  const syncInsights = useCallback((): void => {
    fetchSystemSummary()
      .then((s) => setInsightCount(s.insights.promotions + s.insights.staleOverrides))
      .catch(() => setInsightCount(null));
  }, []);
  useEffect(syncInsights, [syncInsights]);

  // Approvals badge: REST is the source of truth (mount + reconnect resync);
  // the WS stream is the low-latency hint in between (docs/ws-protocol.md).
  const syncPending = useCallback((): void => {
    fetchApprovals('pending')
      .then((list) => setPendingIds(new Set(list.map((r) => r.id))))
      .catch(() => setPendingIds(new Set())); // approvals API absent → no badge
  }, []);
  useEffect(syncPending, [syncPending]);

  const onMessage = useCallback(
    (msg: WSMessage): void => {
      if (msg.type === 'permission_requested') {
        setPendingIds((prev) => new Set(prev).add(msg.payload.id));
      } else if (msg.type === 'permission_resolved') {
        setPendingIds((prev) => {
          if (!prev.has(msg.payload.id)) return prev;
          const next = new Set(prev);
          next.delete(msg.payload.id);
          return next;
        });
      } else if (msg.type === 'system_item_updated') {
        // Registry change → System nav badge resync. The message is rare
        // (scanner/edit events), so no debounce is needed.
        syncInsights();
      }
      // Other message types are the pages' concern — ignore here.
    },
    [syncInsights],
  );
  // Reconnect / 60s reconcile: resync BOTH WS-driven badges — pending
  // approvals and the System insights count — since either may have drifted
  // while the socket was down.
  const resyncBadges = useCallback((): void => {
    syncPending();
    syncInsights();
  }, [syncPending, syncInsights]);
  useLiveUpdates(onMessage, resyncBadges);

  const pendingCount = pendingIds.size;
  const items: NavItem[] = [
    { to: '/', glyph: '◉', label: 'Command deck' },
    { to: '/sessions', glyph: '❯', label: 'Sessions', ...badgeFor(sessionsToday) },
    { to: '/projects', glyph: '▤', label: 'Projects' },
    { to: '/analytics', glyph: '▦', label: 'Analytics' },
    {
      to: '/approvals',
      glyph: '⧗',
      label: 'Approvals',
      ...(pendingCount > 0 ? { badge: String(pendingCount), alert: true } : {}),
    },
    { to: '/system', glyph: '⚙', label: 'System', ...badgeFor(insightCount) },
    ...(hasDocs ? [DOCS_NAV] : []),
  ];

  const daemonOk = !unreachable;

  return (
    <div className="app-shell flex h-dvh flex-col">
      {/* Full-width top header: wordmark left, breadcrumb, live status right. */}
      <header className="header-hairline relative z-20 flex h-14 shrink-0 items-center gap-4 bg-bg px-4 desk:px-6">
        {/* Fixed-width block on desktop: 24px header padding + 208px + 16px gap
            = 248px, so the scope switcher starts exactly where the sidebar ends. */}
        <span className="flex min-w-0 items-center desk:w-[208px] desk:shrink-0">
          <span className="font-sans text-[16px] leading-none font-extrabold tracking-[0.09em] text-ink uppercase">
            SW<span className="text-brand">◆</span>RMERY
          </span>
        </span>
        <ScopeSwitcher />
        {/* Crumb yields to the centered search pill on xl+ (page context is
            already visible in the sidebar highlight). Filter pages skip it
            entirely — their header room belongs to the teleported controls. */}
        {crumb !== '' && !filterPage && (
          <span className="hidden truncate font-mono text-[10.5px] tracking-[0.1em] text-ink-faint uppercase sm:inline xl:hidden">
            {crumb}
          </span>
        )}
        {filterPage ? (
          /* Section-specific controls teleported here by the routed page
             (Sessions: title search + status chips; System: name search +
             level chips) via the HeaderFilters portal. */
          <div id="header-filters" className="hidden min-w-0 flex-1 items-center gap-2 xl:flex" />
        ) : (
          /* Centered search pill — absolutely positioned so it stays in the
             true middle of the header regardless of left/right cluster widths. */
          <button
            type="button"
            onClick={() => setPaletteOpen(true)}
            className="absolute left-1/2 top-1/2 hidden w-[clamp(280px,34vw,480px)] -translate-x-1/2 -translate-y-1/2 items-center justify-between rounded-xl border border-line bg-field px-4 py-2 font-mono text-[12px] text-ink-faint transition-colors hover:border-line-strong hover:text-ink-dim xl:flex"
          >
            <span className="flex items-center gap-2">
              <span aria-hidden="true" className="text-[13px] leading-none">⌕</span>
              search sessions, files, projects…
            </span>
            <span className="rounded-[5px] border border-line-strong px-1.5 py-px text-[10.5px]">⌘K</span>
          </button>
        )}
        <span className="ml-auto flex items-center gap-3">
        {/* Compact fallback where the centered pill is hidden: below xl on
            regular pages, at every width on filter pages (their center hosts
            the teleported filter controls instead). */}
        <button
          type="button"
          onClick={() => setPaletteOpen(true)}
          className={`hidden items-center gap-2 rounded-lg border border-line bg-field px-2.5 py-1 font-mono text-[10.5px] text-ink-faint transition-colors hover:border-line-strong hover:text-ink-dim sm:flex ${filterPage ? '' : 'xl:hidden'}`}
        >
          search <span className="rounded-[4px] border border-line-strong px-1">⌘K</span>
        </button>
        {!MOCK && (
          <span className="flex items-center gap-2">
            <NotifySettings prefs={notifyPrefs} onChange={setNotifyPrefs} />
            <NewProjectButton />
          </span>
        )}
        <span
          className="flex items-center gap-1.5 font-mono text-[10.5px] text-ink-dim"
        >
          {MOCK ? (
            <>
              <span className="inline-block h-[7px] w-[7px] rounded-full bg-amber" />
              mock data
            </>
          ) : (
            <>
              <span
                className={`inline-block h-[7px] w-[7px] rounded-full ${daemonOk ? 'animate-pulse-dot bg-green' : 'bg-red'}`}
              />
              {daemonOk ? 'daemon healthy' : 'daemon unreachable'}
              {health !== null ? ` · ${shortVersion(health.version)}` : ''}
            </>
          )}
        </span>
        </span>
      </header>

      <div className="flex min-h-0 flex-1">
        {/* Desktop sidebar — static labelled panel (248px), no collapse. */}
        <nav className="hidden w-[248px] shrink-0 flex-col gap-0.5 border-r border-line px-3 py-4 desk:flex">
          <div className="flex flex-col gap-0.5">
            {items.map((item) => (
              <NavLink
                key={item.to}
                to={item.to}
                end={item.to === '/'}
                className={({ isActive }) =>
                  `flex h-[38px] items-center gap-3 rounded-[10px] border px-3 transition-colors ${
                    isActive
                      ? 'border-line-strong bg-surface2 text-brand'
                      : 'border-transparent text-ink-dim hover:bg-surface2/50 hover:text-ink'
                  }`
                }
              >
                <span
                  className="w-[16px] shrink-0 text-center text-[16px] leading-none"
                  aria-hidden="true"
                >
                  {item.glyph}
                </span>
                <span className="truncate text-[13.5px] font-medium">{item.label}</span>
                {item.badge !== undefined && (
                  <span
                    className={`ml-auto flex h-[18px] min-w-[18px] items-center justify-center rounded-full px-[5px] font-mono text-[10px] font-bold ${
                      item.alert === true ? 'bg-amber text-bg' : 'bg-line-strong text-ink-dim'
                    }`}
                  >
                    {item.badge}
                  </span>
                )}
              </NavLink>
            ))}
          </div>
        </nav>

        <main className="min-w-0 flex-1 overflow-y-auto pb-[72px] [-webkit-overflow-scrolling:touch] desk:pb-0">
          <Outlet />
        </main>
      </div>

      {/* Mobile bottom nav */}
      <nav className="fixed inset-x-0 bottom-0 z-20 flex justify-around border-t border-line bg-bg/95 px-1 pt-2 pb-[calc(8px+env(safe-area-inset-bottom))] backdrop-blur-md desk:hidden">
        {items.map((item) => (
          <NavLink
            key={item.to}
            to={item.to}
            end={item.to === '/'}
            className={({ isActive }) =>
              `flex flex-col items-center gap-[3px] rounded-lg px-2.5 py-1 text-[10.5px] transition-colors ${
                isActive ? 'font-medium text-brand' : 'text-ink-faint hover:text-ink'
              }`
            }
          >
            <span className="relative text-[17px] leading-none" aria-hidden="true">
              {item.glyph}
              {item.alert === true && (
                <span className="absolute -top-0.5 -right-1.5 h-[6px] w-[6px] rounded-full bg-amber" />
              )}
            </span>
            {item.label}
          </NavLink>
        ))}
      </nav>

      {paletteOpen && <CommandPalette onClose={() => setPaletteOpen(false)} />}
    </div>
  );
}

/** Neutral (non-alert) count badge when a positive number is available. */
function badgeFor(n: number | null): { badge?: string } {
  return n !== null && n > 0 ? { badge: String(n) } : {};
}
