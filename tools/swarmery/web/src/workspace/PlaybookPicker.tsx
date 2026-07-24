// Playbook picker (fusion phase 13): a <select> of the project's playbooks,
// shared by QuickEntry (compact) and TaskDrawer (with a description line). The
// list is fetched once per project (built-ins overlaid by project overrides);
// the empty value maps to the default recipe ('standard'). Fully keyboard-native
// (a plain <select>), labelled for WCAG.

import { useEffect, useState } from 'react';
import type { Playbook } from '../api/types';
import { fetchPlaybooks } from '../api';

/** The recipe used when no playbook is selected — its selection maps to "". */
export const DEFAULT_PLAYBOOK = 'standard';

/** Shared fetch: the playbooks visible to a project, or [] on error. */
export function usePlaybooks(projectId: number | null): {
  playbooks: Playbook[];
  loading: boolean;
} {
  const [playbooks, setPlaybooks] = useState<Playbook[]>([]);
  const [loading, setLoading] = useState(true);
  useEffect(() => {
    if (projectId === null) {
      setPlaybooks([]);
      setLoading(false);
      return;
    }
    let disposed = false;
    setLoading(true);
    fetchPlaybooks(projectId)
      .then((ps) => {
        if (!disposed) setPlaybooks(ps);
      })
      .catch(() => {
        if (!disposed) setPlaybooks([]);
      })
      .finally(() => {
        if (!disposed) setLoading(false);
      });
    return () => {
      disposed = true;
    };
  }, [projectId]);
  return { playbooks, loading };
}

/**
 * A <select> bound to a playbook name. `value` is the selected name ("" = the
 * default recipe); `onChange` receives the new name ("" when the default is
 * picked). `compact` shrinks it for the QuickEntry row.
 */
export function PlaybookSelect({
  playbooks,
  value,
  onChange,
  compact = false,
  disabled = false,
  id,
}: {
  playbooks: Playbook[];
  value: string;
  onChange: (name: string) => void;
  compact?: boolean;
  disabled?: boolean;
  id?: string;
}): JSX.Element {
  // The default recipe is offered as the empty option; every other playbook is a
  // named option. If the current value is a name not in the list (e.g. a stored
  // project playbook the fetch has not returned yet) it still renders selected.
  const known = playbooks.some((p) => p.name === value);
  const base = compact
    ? 'rounded-md border border-line bg-field px-1.5 py-1 font-mono text-[10px] text-ink-dim'
    : 'w-full rounded-[8px] border border-line bg-field px-2 py-1.5 font-mono text-[11px] text-ink';
  return (
    <select
      id={id}
      value={value}
      disabled={disabled}
      aria-label="playbook"
      onClick={(e) => e.stopPropagation()}
      onChange={(e) => onChange(e.target.value)}
      className={`${base} outline-none transition-colors hover:border-line-strong focus:border-ink-dim disabled:opacity-50`}
    >
      <option value="">{compact ? 'standard' : 'standard (default)'}</option>
      {value !== '' && !known && <option value={value}>{value}</option>}
      {playbooks
        .filter((p) => p.name !== DEFAULT_PLAYBOOK)
        .map((p) => (
          <option key={p.name} value={p.name}>
            {p.name}
            {p.source === 'project' ? ' •' : ''}
          </option>
        ))}
    </select>
  );
}

/** The verify chip + one-line description for the selected playbook (drawer). */
export function PlaybookHint({
  playbooks,
  value,
}: {
  playbooks: Playbook[];
  value: string;
}): JSX.Element | null {
  const name = value === '' ? DEFAULT_PLAYBOOK : value;
  const pb = playbooks.find((p) => p.name === name);
  if (pb === undefined) return null;
  return (
    <div className="mt-1 flex items-start gap-1.5">
      <VerifyChip verify={pb.verify} />
      <span className="font-mono text-[10px] leading-snug text-ink-faint">{pb.description}</span>
    </div>
  );
}

const VERIFY_CHIP: Record<string, string> = {
  strict: 'border-amber/40 bg-amber/10 text-amber',
  normal: 'border-line text-ink-dim',
  off: 'border-line-strong text-ink-faint',
};

/** Small chip showing a playbook's verify strictness. */
export function VerifyChip({ verify }: { verify: string }): JSX.Element {
  const style = VERIFY_CHIP[verify] ?? 'border-line text-ink-faint';
  return (
    <span
      className={`shrink-0 rounded-full border px-1.5 py-[1px] font-mono text-[9px] uppercase ${style}`}
      title={`verification: ${verify}`}
    >
      verify {verify}
    </span>
  );
}
