// Insights tab (System screen): the promotion & drift detector — read-only
// advisory cards over GET /api/system/insights. Three sections: promotion
// candidates (graduation rule, docs/EXTENDING.md), stale local overrides
// (local name colliding with a plugin item), dead components (active
// agent_dead findings). Each item expands to copies + a redacted unified
// diff (DiffBlock, shared with the detail panel) + a copyable next-step
// hint. Display-only by design — promotion itself stays a manual flow.

import { useEffect, useState } from 'react';
import type {
  SystemDeadComponent,
  SystemInsights,
  SystemPromotionCandidate,
  SystemStaleOverride,
} from '../../api/types';
import { fetchSystemInsights } from '../../api/system';
import { Empty, ErrorBox, Loading } from '../../components/ui';
import { useProjectColor } from '../../lib/projectColors';
import { DiffBlock } from './ItemDetail';
import { ScopeBadge } from './shared';

/* ----- shared atoms ----- */

function KindBadge({ kind }: { kind: string }): JSX.Element {
  return (
    <span className="shrink-0 rounded-full border border-line-strong px-2 py-px font-mono text-[10px] whitespace-nowrap text-ink-dim">
      {kind}
    </span>
  );
}

function SimilarityChip({
  identical,
  stat,
}: {
  identical: boolean;
  stat: { added: number; removed: number } | null;
}): JSX.Element {
  if (identical) {
    return (
      <span className="shrink-0 rounded-full border border-green/40 px-2 py-px font-mono text-[10px] whitespace-nowrap text-green">
        identical
      </span>
    );
  }
  return (
    <span className="shrink-0 rounded-full border border-amber/40 px-2 py-px font-mono text-[10px] whitespace-nowrap text-amber">
      diverged{stat !== null ? ` +${String(stat.added)}/−${String(stat.removed)}` : ''}
    </span>
  );
}

function ProjectChip({ slug }: { slug: string | null }): JSX.Element {
  // App-wide distinct-color map (falls back to the per-slug hash off-list).
  const colorFor = useProjectColor();
  if (slug === null) {
    return <span className="font-mono text-[10.5px] text-ink-dim">global</span>;
  }
  return (
    <span className="font-mono text-[10.5px]" style={{ color: colorFor(slug) }}>
      {slug}
    </span>
  );
}

/** Copyable next-step hint — display-only, no write actions (YAGNI). */
function HintLine({ hint }: { hint: string }): JSX.Element {
  const [copied, setCopied] = useState(false);
  return (
    <div className="mt-2 flex items-start gap-2 rounded-lg border border-line bg-bg px-3 py-2">
      <code className="min-w-0 flex-1 font-mono text-[11px] whitespace-pre-wrap text-ink-dim">
        {hint}
      </code>
      <button
        type="button"
        aria-label="copy next-step hint"
        onClick={() => {
          // navigator.clipboard is undefined on non-secure origins (plain-HTTP
          // LAN) — optional-chain to a no-op instead of throwing; the hint
          // text itself stays visible/selectable either way.
          void navigator.clipboard
            ?.writeText(hint)
            .then(() => {
              setCopied(true);
              setTimeout(() => setCopied(false), 1500);
            })
            .catch(() => {});
        }}
        className="shrink-0 rounded border border-line-strong px-2 py-0.5 font-mono text-[10px] text-ink-dim transition-colors hover:text-ink"
      >
        {copied ? 'copied' : 'copy'}
      </button>
    </div>
  );
}

function ExpandableRow({
  header,
  children,
}: {
  header: JSX.Element;
  children: React.ReactNode;
}): JSX.Element {
  const [open, setOpen] = useState(false);
  return (
    <div className="border-b border-line-soft last:border-b-0">
      <button
        type="button"
        onClick={() => setOpen((v) => !v)}
        aria-expanded={open}
        className="flex w-full items-center gap-2 px-3.5 py-2.5 text-left transition-colors hover:bg-surface2/50"
      >
        <span aria-hidden="true" className="w-3 shrink-0 font-mono text-[10px] text-ink-faint">
          {open ? '▾' : '▸'}
        </span>
        {header}
      </button>
      {open && <div className="px-3.5 pb-3 pl-[34px]">{children}</div>}
    </div>
  );
}

function InsightSection({
  title,
  count,
  subtitle,
  children,
}: {
  title: string;
  count: number;
  subtitle: string;
  children: React.ReactNode;
}): JSX.Element {
  return (
    <section className="overflow-hidden rounded-xl border border-line bg-surface">
      <div className="flex items-baseline gap-2 border-b border-line px-3.5 py-2.5">
        <h2 className="text-[13.5px] font-semibold text-ink">{title}</h2>
        <span className="font-mono text-[11px] text-ink-dim">{String(count)}</span>
        <span className="ml-auto hidden font-mono text-[10px] text-ink-faint sm:inline">
          {subtitle}
        </span>
      </div>
      {children}
    </section>
  );
}

/* ----- per-category rows ----- */

function PromotionRow({ c }: { c: SystemPromotionCandidate }): JSX.Element {
  return (
    <ExpandableRow
      header={
        <>
          <KindBadge kind={c.kind} />
          <span className="min-w-0 truncate text-[13.5px] font-semibold text-ink">{c.name}</span>
          <span className="flex min-w-0 items-center gap-1.5 truncate">
            {c.copies.map((copy) => (
              <ProjectChip key={copy.itemId} slug={copy.projectSlug} />
            ))}
          </span>
          <span className="ml-auto">
            <SimilarityChip identical={c.similarity === 'identical'} stat={c.diffStat} />
          </span>
        </>
      }
    >
      <div className="space-y-0.5">
        {c.copies.map((copy) => (
          <div key={copy.itemId} className="flex items-center gap-2 font-mono text-[10.5px] text-ink-faint">
            <ProjectChip slug={copy.projectSlug} />
            <span className="min-w-0 truncate">{copy.path}</span>
          </div>
        ))}
      </div>
      {c.diff !== '' && <DiffBlock diff={c.diff} />}
      {c.similarity === 'identical' && (
        <div className="mt-1 font-mono text-[11px] text-green">
          all copies share one content hash — a clean promotion, no reconciliation needed
        </div>
      )}
      <HintLine hint={c.hint} />
    </ExpandableRow>
  );
}

function OverrideRow({ o }: { o: SystemStaleOverride }): JSX.Element {
  return (
    <ExpandableRow
      header={
        <>
          <KindBadge kind={o.kind} />
          <span className="min-w-0 truncate text-[13.5px] font-semibold text-ink">{o.name}</span>
          <span className="shrink-0 rounded-full border border-brand/40 px-2 py-px font-mono text-[10px] whitespace-nowrap text-brand">
            plugin · {o.pluginName}
          </span>
          <ProjectChip slug={o.local.projectSlug} />
          <span className="ml-auto">
            <SimilarityChip identical={o.identical} stat={o.diffStat} />
          </span>
        </>
      }
    >
      <div className="space-y-0.5 font-mono text-[10.5px] text-ink-faint">
        <div className="flex items-center gap-2">
          <span className="w-[42px] shrink-0 text-ink-dim">local</span>
          <span className="min-w-0 truncate">{o.local.path}</span>
        </div>
        <div className="flex items-center gap-2">
          <span className="w-[42px] shrink-0 text-ink-dim">plugin</span>
          <span className="min-w-0 truncate">{o.plugin.path}</span>
        </div>
      </div>
      {o.diff !== '' && <DiffBlock diff={o.diff} />}
      <HintLine hint={o.hint} />
    </ExpandableRow>
  );
}

function DeadRow({ d }: { d: SystemDeadComponent }): JSX.Element {
  return (
    <ExpandableRow
      header={
        <>
          <KindBadge kind={d.kind} />
          <span className="min-w-0 truncate text-[13.5px] font-semibold text-ink">{d.name}</span>
          <span className="ml-auto">
            <ScopeBadge scope={d.scope} projectSlug={d.projectSlug} />
          </span>
        </>
      }
    >
      <div className="font-mono text-[11px] text-ink-dim">{d.message}</div>
      <HintLine hint={d.hint} />
    </ExpandableRow>
  );
}

/* ----- the tab ----- */

export function InsightsTab({ refreshKey }: { refreshKey: number }): JSX.Element {
  const [insights, setInsights] = useState<SystemInsights | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [attempt, setAttempt] = useState(0);

  useEffect(() => {
    let cancelled = false;
    fetchSystemInsights()
      .then((data) => {
        if (cancelled) return;
        setInsights(data);
        setError(null);
      })
      .catch((e: unknown) => {
        if (!cancelled) setError(String(e));
      });
    return () => {
      cancelled = true;
    };
  }, [refreshKey, attempt]);

  if (error !== null) return <ErrorBox message={error} onRetry={() => setAttempt((a) => a + 1)} />;
  if (insights === null) return <Loading label="insights…" />;

  return (
    <div className="space-y-4">
      <InsightSection
        title="Promotion candidates"
        count={insights.promotionCandidates.length}
        subtitle="same-named local component in ≥2 projects — graduation rule (EXTENDING.md)"
      >
        {insights.promotionCandidates.length === 0 ? (
          <Empty>no component is duplicated across projects — nothing to promote</Empty>
        ) : (
          insights.promotionCandidates.map((c) => <PromotionRow key={`${c.kind}:${c.name}`} c={c} />)
        )}
      </InsightSection>

      <InsightSection
        title="Stale local overrides"
        count={insights.staleOverrides.length}
        subtitle="local name colliding with a plugin item — identical copies are safe to delete"
      >
        {insights.staleOverrides.length === 0 ? (
          <Empty>no local copy shadows a plugin component</Empty>
        ) : (
          insights.staleOverrides.map((o) => (
            <OverrideRow key={`${o.kind}:${o.local.itemId}:${o.plugin.itemId}`} o={o} />
          ))
        )}
      </InsightSection>

      <InsightSection
        title="Dead components"
        count={insights.dead.length}
        subtitle="0 telemetry mentions in 30 days (advisory)"
      >
        {insights.dead.length === 0 ? (
          <Empty>every agent has recent telemetry mentions</Empty>
        ) : (
          insights.dead.map((d) => <DeadRow key={d.id} d={d} />)
        )}
      </InsightSection>
    </div>
  );
}
