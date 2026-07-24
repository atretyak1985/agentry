// Quick-entry (fusion phase 4): an inline input at the top of the Triage
// column. Typing a title and pressing Enter creates a Triage task in one
// keystroke flow (POST /api/board/tasks with prompt=title for now — the full
// editor is the drawer). No modal (Fusion's intake-column pattern).

import { useState } from 'react';
import type { BoardTask } from '../api/types';
import { createBoardTask } from '../api';

export function QuickEntry({
  projectId,
  onCreated,
}: {
  projectId: number;
  /** Called with the created row so the board can insert it optimistically. */
  onCreated: (task: BoardTask) => void;
}): JSX.Element {
  const [title, setTitle] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const submit = (): void => {
    const t = title.trim();
    if (t === '' || busy) return;
    setBusy(true);
    setError(null);
    createBoardTask({ projectId, title: t, prompt: t })
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
      {error !== null && <div className="mt-1 font-mono text-[10px] text-red">{error}</div>}
    </div>
  );
}
