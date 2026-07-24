// Quick-entry (fusion phase 4): an inline input at the top of the Triage
// column. Typing a title and pressing Enter creates a Triage task in one
// keystroke flow (POST /api/board/tasks with prompt=title for now — the full
// editor is the drawer). No modal (Fusion's intake-column pattern). Phase 13
// adds a compact playbook selector so a task can pick a recipe at intake.

import { useEffect, useRef, useState } from 'react';
import type { BoardTask } from '../api/types';
import { createBoardTask } from '../api';
import { PlaybookSelect, usePlaybooks } from './PlaybookPicker';

export function QuickEntry({
  projectId,
  onCreated,
  initialTitle = '',
}: {
  projectId: number;
  /** Called with the created row so the board can insert it optimistically. */
  onCreated: (task: BoardTask) => void;
  /** Seed value (Agent Hub "Run now" prefills `@<agent>: `); autofocuses when
   * non-empty. Uncontrolled after mount — editing/submitting clears it. */
  initialTitle?: string;
}): JSX.Element {
  const [title, setTitle] = useState(initialTitle);
  const [playbook, setPlaybook] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const { playbooks } = usePlaybooks(projectId);
  const inputRef = useRef<HTMLInputElement>(null);

  // A prefilled entry (Run now) focuses with the caret at the end so the user
  // types the request right after "@agent: ". One-shot on mount.
  useEffect(() => {
    if (initialTitle !== '' && inputRef.current !== null) {
      const el = inputRef.current;
      el.focus();
      el.setSelectionRange(el.value.length, el.value.length);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const submit = (): void => {
    const t = title.trim();
    if (t === '' || busy) return;
    setBusy(true);
    setError(null);
    createBoardTask({
      projectId,
      title: t,
      prompt: t,
      ...(playbook !== '' ? { playbook } : {}),
    })
      .then((task) => {
        onCreated(task);
        setTitle('');
      })
      .catch((e: unknown) => setError(e instanceof Error ? e.message : String(e)))
      .finally(() => setBusy(false));
  };

  return (
    <div>
      <input
        ref={inputRef}
        type="text"
        value={title}
        disabled={busy}
        onChange={(e) => setTitle(e.target.value)}
        onKeyDown={(e) => {
          if (e.key === 'Enter') {
            e.preventDefault();
            submit();
          }
        }}
        placeholder="+ new task…"
        aria-label="new task title"
        className="w-full rounded-lg border border-dashed border-line bg-transparent px-2.5 py-2 text-[12px] text-ink outline-none transition-colors placeholder:text-ink-faint focus:border-ink-dim focus:bg-field disabled:opacity-50"
      />
      {title.trim() !== '' && (
        <div className="mt-1 flex items-center gap-1.5">
          <span className="font-mono text-[9px] tracking-[0.08em] text-ink-faint uppercase">recipe</span>
          <PlaybookSelect playbooks={playbooks} value={playbook} onChange={setPlaybook} compact disabled={busy} />
        </div>
      )}
      {error !== null && <div className="mt-1 font-mono text-[10px] text-red">{error}</div>}
    </div>
  );
}
