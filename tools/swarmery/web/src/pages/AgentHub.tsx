// Agent Hub (fusion phase 17): agent-centric roster + per-agent tabbed profile,
// aggregating what already exists across System (definition/versions), Retro
// (scorecards/lessons/proposals), Analytics (cost) and Sessions (runs). Built on
// the reusable HubShell (pages/agent-hub/HubShell.tsx) so the System Hub (phase
// 18) can extend the same split-pane pattern.
//
// Routing: /agents (fleet roster) + /agents/:id (fleet, agent selected), and the
// workspace mirror /p/:slug/agents(/:id). The selected agent is the :id route
// param; the active tab is ?tab=. Definition editing is NOT reimplemented — the
// Definition tab embeds the existing System editor (SystemItemPanel), which owns
// the versioned write surface (edit / versions / diff / rollback).

import { useCallback, useEffect, useMemo, useState } from 'react';
import { useNavigate, useParams, useSearchParams } from 'react-router-dom';
import type { AgentProfile, AgentRosterRow, WSMessage } from '../api/types';
import { fetchAgentProfile, fetchAgentRoster } from '../api/agentHub';
import { fetchProjects } from '../api';
import { fmtAgo, fmtCost } from '../lib/format';
import { useScope } from '../lib/scope';
import { useLiveUpdates } from '../lib/ws';
import { Empty, ErrorBox, Loading } from '../components/ui';
import { SystemItemPanel } from './system/ItemDetail';
import { HubShell, healthTone, type HubTab } from './agent-hub/HubShell';
import { ActivityTab, InsightsTab, OverviewTab, RunsTab, TasksTab } from './agent-hub/Tabs';
import { RunNowButton } from './agent-hub/RunNow';

type ProfileTab = 'overview' | 'runs' | 'tasks' | 'activity' | 'insights' | 'definition';
const TABS: ProfileTab[] = ['overview', 'runs', 'tasks', 'activity', 'insights', 'definition'];
const TAB_LABELS: Record<ProfileTab, string> = {
  overview: 'Overview',
  runs: 'Runs',
  tasks: 'Tasks',
  activity: 'Activity',
  insights: 'Insights',
  definition: 'Definition',
};

function parseTab(value: string | null): ProfileTab {
  return (TABS as string[]).includes(value ?? '') ? (value as ProfileTab) : 'overview';
}

/* ----- roster card ----- */

function RosterCard({ agent }: { agent: AgentRosterRow }): JSX.Element {
  const health = healthTone(agent.failedShare);
  return (
    <>
      <div className="flex flex-wrap items-center gap-2">
        <span
          className={`inline-block h-[8px] w-[8px] shrink-0 rounded-full ${health.dot}`}
          title={`${health.label} · ${Math.round(agent.failedShare * 100)}% failed-run share`}
        />
        <span className="text-[13.5px] font-semibold text-ink">{agent.name}</span>
        {agent.model !== null && (
          <span className="font-mono text-[10px] text-ink-faint">{agent.model}</span>
        )}
        <span className="ml-auto rounded-[6px] border border-line-strong px-1.5 py-[1px] font-mono text-[9.5px] text-ink-dim">
          {agent.scope}
        </span>
      </div>
      <div className="mt-1.5 flex flex-wrap items-center gap-x-3 font-mono text-[10px] text-ink-faint">
        <span className="whitespace-nowrap">runs 30d {String(agent.runs30d)}</span>
        <span className="whitespace-nowrap">{fmtCost(agent.cost30d)}</span>
        <span className="whitespace-nowrap">
          {agent.lastActiveAt !== null ? `active ${fmtAgo(agent.lastActiveAt)}` : 'idle'}
        </span>
      </div>
    </>
  );
}

/* ----- detail header (identity + Run now + Definition link) ----- */

function ProfileHeader({
  agent,
  scopeSlug,
}: {
  agent: AgentProfile;
  scopeSlug: string | null;
}): JSX.Element {
  const health = healthTone(agent.failedShare);
  return (
    <div className="flex flex-wrap items-center gap-x-3 gap-y-2">
      <span className={`inline-block h-[9px] w-[9px] shrink-0 rounded-full ${health.dot}`} />
      <span className="font-display text-[18px] leading-none font-medium text-ink">{agent.name}</span>
      {agent.model !== null && (
        <span className="font-mono text-[11px] text-ink-dim">{agent.model}</span>
      )}
      <span className="font-mono text-[10px] text-ink-faint">{health.label}</span>
      <span className="ml-auto flex items-center gap-2">
        <RunNowButton agentName={agent.name} scopeSlug={scopeSlug} />
      </span>
    </div>
  );
}

/* ----- the page ----- */

export function AgentHub(): JSX.Element {
  const params = useParams();
  const navigate = useNavigate();
  const [searchParams, setSearchParams] = useSearchParams();
  const { scope } = useScope();
  // Workspace mount (/p/:slug/agents) carries the slug in the route; fleet mount
  // uses the global scope switcher. Either narrows the rollup window.
  const scopeSlug = params.slug ?? scope;
  const routeBase = params.slug !== undefined ? `/p/${params.slug}/agents` : '/agents';

  const selectedId = params.id !== undefined && /^\d+$/.test(params.id) ? Number(params.id) : null;
  const tab = parseTab(searchParams.get('tab'));

  const [roster, setRoster] = useState<AgentRosterRow[] | null>(null);
  const [rosterError, setRosterError] = useState<string | null>(null);
  const [profile, setProfile] = useState<AgentProfile | null>(null);
  const [profileError, setProfileError] = useState<string | null>(null);
  const [defRefresh, setDefRefresh] = useState(0);
  const [projectNames, setProjectNames] = useState<Record<string, string>>({});

  const loadRoster = useCallback((): void => {
    setRosterError(null);
    fetchAgentRoster(scopeSlug ?? undefined)
      .then((r) => setRoster(r.agents))
      .catch((e: unknown) => setRosterError(String(e)));
  }, [scopeSlug]);
  useEffect(loadRoster, [loadRoster]);

  useEffect(() => {
    fetchProjects()
      .then((ps) => setProjectNames(Object.fromEntries(ps.map((p) => [p.slug, p.name ?? p.slug]))))
      .catch(() => setProjectNames({}));
  }, []);

  const loadProfile = useCallback((): void => {
    if (selectedId === null) {
      setProfile(null);
      return;
    }
    setProfileError(null);
    fetchAgentProfile(selectedId, scopeSlug ?? undefined)
      .then(setProfile)
      .catch((e: unknown) => setProfileError(String(e)));
  }, [selectedId, scopeSlug]);
  useEffect(loadProfile, [loadProfile]);

  // Live: a registry edit (WS system_item_updated) refetches the roster, the
  // profile, and bumps the embedded Definition editor — the same invalidation
  // hint the System page uses (payload carries ids only).
  const onMessage = useCallback(
    (msg: WSMessage): void => {
      if (msg.type === 'system_item_updated') {
        loadRoster();
        loadProfile();
        setDefRefresh((k) => k + 1);
      }
    },
    [loadRoster, loadProfile],
  );
  const resync = useCallback((): void => {
    loadRoster();
    loadProfile();
  }, [loadRoster, loadProfile]);
  useLiveUpdates(onMessage, resync);

  const onSelect = useCallback(
    (key: string | null): void => {
      navigate(key === null ? routeBase : `${routeBase}/${key}${window.location.search}`);
    },
    [navigate, routeBase],
  );
  const onTab = useCallback(
    (id: string): void => {
      setSearchParams(
        (prev) => {
          const next = new URLSearchParams(prev);
          if (id === 'overview') next.delete('tab');
          else next.set('tab', id);
          return next;
        },
        { replace: true },
      );
    },
    [setSearchParams],
  );

  const tabs: HubTab[] = useMemo(
    () =>
      TABS.map((t) => {
        const badge =
          t === 'insights' && profile !== null
            ? profile.insights.recommendations.length +
              profile.insights.proposals.length +
              profile.insights.lessons.length
            : t === 'runs' && profile !== null
              ? profile.runs.length
              : undefined;
        return { id: t, label: TAB_LABELS[t], ...(badge !== undefined ? { badge } : {}) };
      }),
    [profile],
  );

  const rowMatches = useCallback(
    (a: AgentRosterRow, q: string): boolean =>
      [a.name, a.model, a.description].some((v) => v != null && v.toLowerCase().includes(q)),
    [],
  );

  return (
    <HubShell<AgentRosterRow>
      title="Agents"
      roster={roster}
      rosterError={rosterError}
      onRosterRetry={loadRoster}
      rowKey={(a) => String(a.id)}
      rowMatches={rowMatches}
      renderRow={(a) => <RosterCard agent={a} />}
      selectedKey={selectedId === null ? null : String(selectedId)}
      onSelect={onSelect}
      searchPlaceholder="filter agents…"
      rosterEmptyLabel="no agents on this machine"
      tabs={tabs}
      activeTab={tab}
      onTab={onTab}
      detailHeader={profile !== null ? <ProfileHeader agent={profile} scopeSlug={scopeSlug} /> : undefined}
      detailPlaceholder={<Empty>select an agent to see its profile</Empty>}
    >
      {selectedId !== null && (
        <ProfilePanel
          tab={tab}
          selectedId={selectedId}
          profile={profile}
          profileError={profileError}
          onProfileRetry={loadProfile}
          projectNames={projectNames}
          scopeSlug={scopeSlug}
          defRefresh={defRefresh}
          onDefinitionMutated={() => {
            setDefRefresh((k) => k + 1);
            loadRoster();
            loadProfile();
          }}
        />
      )}
    </HubShell>
  );
}

/** The active tab's panel. Definition mounts the existing System editor; every
 * other tab renders a slice of the aggregated profile bundle. */
function ProfilePanel({
  tab,
  selectedId,
  profile,
  profileError,
  onProfileRetry,
  projectNames,
  scopeSlug,
  defRefresh,
  onDefinitionMutated,
}: {
  tab: ProfileTab;
  selectedId: number;
  profile: AgentProfile | null;
  profileError: string | null;
  onProfileRetry: () => void;
  projectNames: Record<string, string>;
  scopeSlug: string | null;
  defRefresh: number;
  onDefinitionMutated: () => void;
}): JSX.Element {
  // Definition tab: reuse the existing versioned System editor verbatim. It
  // fetches its own detail by the SAME registry id, so it works standalone —
  // create/delete stay on the System page (this is edit/versions/rollback only).
  if (tab === 'definition') {
    return (
      <SystemItemPanel
        kind="agents"
        id={selectedId}
        refreshKey={defRefresh}
        projectNames={projectNames}
        onClose={() => undefined}
        onMutated={onDefinitionMutated}
        onDeleted={onDefinitionMutated}
        onReadonly={() => undefined}
      />
    );
  }

  if (profileError !== null) return <ErrorBox message={profileError} onRetry={onProfileRetry} />;
  if (profile === null) return <Loading label="profile…" />;

  switch (tab) {
    case 'runs':
      return <RunsTab runs={profile.runs} />;
    case 'tasks':
      return <TasksTab tasks={profile.tasks} projectSlug={scopeSlug} />;
    case 'activity':
      return <ActivityTab activity={profile.activity} />;
    case 'insights':
      return <InsightsTab insights={profile.insights} />;
    default:
      return (
        <OverviewTab overview={profile.overview} topInsights={profile.insights.recommendations} />
      );
  }
}
