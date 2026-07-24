// Board task card (fusion phase 4): the draggable unit on the kanban board.
// Shows title, the T-xxxx id chip, a priority dot, a model chip (when set), a
// verdict badge (pass/fail/inconclusive — phase 6 fills verifyVerdict), a
// dispatch-error warning icon with tooltip, a paused badge, and a session link
// glyph when a branch/worktree exists. Native HTML5 draggable; a keyboard
// alternative (a "move to →" menu) lives on the card via ColumnMenu so drag is
// never the only path (WCAG). Clicking the card body opens the TaskDrawer.

import type { BoardColumn, BoardTask, TaskPriority } from '../api/types';
import { BOARD_COLUMNS, COLUMN_LABELS } from './boardModel';

const PRIORITY_DOT: Record<TaskPriority, string> = {
  urgent: 'bg-red',
  high: 'bg-amber',
  normal: 'bg-ink-faint',
  low: 'bg-line-strong',
};

const VERDICT_STYLE: Record<string, string> = {
  pass: 'border-green/40 bg-green/10 text-green',
  fail: 'border-red/40 bg-red/10 text-red',
  inconclusive: 'border-amber/40 bg-amber/10 text-amber',
};

function VerdictBadge({ verdict }: { verdict: string }): JSX.Element {
  const style = VERDICT_STYLE[verdict] ?? 'border-line text-ink-faint';
  return (
    <span className={`rounded-full border px-1.5 py-[1px] font-mono text-[9px] uppercase ${style}`}>
      {verdict}
    </span>
  );
}

/** Keyboard alternative to drag: a native <select> that moves the card. Sits
 * on every card so column changes never require a pointer. */
function ColumnMenu({
  column,
  onMove,
}: {
  column: BoardColumn;
  onMove: (to: BoardColumn) => void;
}): JSX.Element {
  return (
    <select
      value={column}
      aria-label="move task to column"
      onClick={(e) => e.stopPropagation()}
      onChange={(e) => {
        const to = e.target.value as BoardColumn;
        if (to !== column) onMove(to);
      }}
      className="rounded-md border border-line bg-field px-1 py-[1px] font-mono text-[9.5px] text-ink-dim outline-none transition-colors hover:border-line-strong focus:border-ink-dim"
    >
      {BOARD_COLUMNS.map((c) => (
        <option key={c} value={c}>
          {COLUMN_LABELS[c]}
        </option>
      ))}
    </select>
  );
}

export function TaskCard({
  task,
  onOpen,
  onMove,
  onDragStart,
  onDragEnd,
  dragging,
}: {
  task: BoardTask;
  onOpen: () => void;
  onMove: (to: BoardColumn) => void;
  onDragStart: () => void;
  onDragEnd: () => void;
  dragging: boolean;
}): JSX.Element {
  const blocked = task.paused || task.userPaused;
  return (
    <div
      draggable
      onDragStart={(e) => {
        e.dataTransfer.setData('text/plain', String(task.id));
        e.dataTransfer.effectAllowed = 'move';
        onDragStart();
      }}
      onDragEnd={onDragEnd}
      role="button"
      tabIndex={0}
      aria-label={`task ${task.externalId}: ${task.title}`}
      onClick={onOpen}
      onKeyDown={(e) => {
        if (e.key === 'Enter' || e.key === ' ') {
          e.preventDefault();
          onOpen();
        }
      }}
      className={`group cursor-pointer rounded-lg border bg-surface p-2.5 transition-colors hover:border-line-strong focus:border-ink-dim focus:outline-none ${
        dragging ? 'border-ink-dim opacity-40' : 'border-line'
      }`}
    >
      <div className="flex items-start gap-2">
        <span
          aria-hidden="true"
          title={`${task.priority} priority`}
          className={`mt-[5px] h-[7px] w-[7px] shrink-0 rounded-full ${PRIORITY_DOT[task.priority]}`}
        />
        <span className="min-w-0 flex-1 text-[12.5px] leading-snug text-ink">{task.title}</span>
        {task.dispatchError !== null && (
          <span
            aria-label={`dispatch error: ${task.dispatchError}`}
            title={task.dispatchError}
            className="shrink-0 text-[12px] leading-none text-red"
          >
            ⚠
          </span>
        )}
      </div>

      <div className="mt-2 flex flex-wrap items-center gap-1.5">
        <span className="rounded border border-line px-1 py-[1px] font-mono text-[9px] text-ink-faint">
          {task.externalId}
        </span>
        {task.model !== null && (
          <span className="rounded border border-line px-1 py-[1px] font-mono text-[9px] text-ink-dim">
            {task.model}
          </span>
        )}
        {task.verifyVerdict !== null && <VerdictBadge verdict={task.verifyVerdict} />}
        {blocked && (
          <span className="rounded-full border border-amber/40 bg-amber/10 px-1.5 py-[1px] font-mono text-[9px] text-amber">
            paused
          </span>
        )}
        {task.branch !== null && (
          <a
            href={`/sessions?scope=${task.projectSlug ?? ''}`}
            onClick={(e) => e.stopPropagation()}
            title={`branch ${task.branch}`}
            aria-label={`sessions for ${task.branch}`}
            className="font-mono text-[9px] text-ink-faint transition-colors hover:text-ink"
          >
            ❯ session
          </a>
        )}
        <span className="ml-auto opacity-0 transition-opacity group-hover:opacity-100 group-focus-within:opacity-100">
          <ColumnMenu column={task.boardColumn} onMove={onMove} />
        </span>
      </div>
    </div>
  );
}
