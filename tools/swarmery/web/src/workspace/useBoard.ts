// Board state hook (fusion phase 4): owns the project's board-task list, its
// live WS patching, and the optimistic column-move with revert-on-error. The
// Board page, StatusBar, and TaskDrawer all read the same instance passed down
// as props so there is one source of truth per workspace mount.
//
// Liveness: subscribes to the shared WS via useLiveUpdates → task_updated
// patches the list in place (applyBoardTaskMessage), never a refetch; the 60s
// reconcile + reconnect refetches the whole list as the convergence net.

import { useCallback, useEffect, useRef, useState } from 'react';
import type { BoardColumn, BoardTask, WSMessage } from '../api/types';
import { fetchBoardTasks, patchBoardTask, type PatchBoardTaskInput } from '../api';
import { applyBoardTaskMessage, useLiveUpdates } from '../lib/ws';

export interface BoardState {
  tasks: BoardTask[];
  loading: boolean;
  error: string | null;
  /** Transient action error (a failed move/edit) — shown as a dismissable toast. */
  actionError: string | null;
  clearActionError: () => void;
  reload: () => void;
  /** Optimistic column move; reverts + sets actionError on a 4xx/5xx. */
  moveTask: (id: number, to: BoardColumn) => void;
  /** Optimistic field edit (drawer). Resolves to the server row or rejects. */
  patchTask: (id: number, patch: PatchBoardTaskInput) => Promise<BoardTask>;
  /** Insert a freshly-created task at the head (QuickEntry). */
  addTask: (task: BoardTask) => void;
}

export function useBoard(projectId: number | null): BoardState {
  const [tasks, setTasks] = useState<BoardTask[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);
  const aliveRef = useRef(true);

  useEffect(() => {
    aliveRef.current = true;
    return () => {
      aliveRef.current = false;
    };
  }, []);

  const reload = useCallback((): void => {
    if (projectId === null) {
      setTasks([]);
      setLoading(false);
      return;
    }
    fetchBoardTasks(projectId)
      .then((rows) => {
        if (!aliveRef.current) return;
        setTasks(rows);
        setError(null);
        setLoading(false);
      })
      .catch((e: unknown) => {
        if (!aliveRef.current) return;
        setError(e instanceof Error ? e.message : String(e));
        setLoading(false);
      });
  }, [projectId]);

  useEffect(() => {
    setLoading(true);
    reload();
  }, [reload]);

  // Live board: patch by id in place; reconcile refetches the whole list.
  const onMessage = useCallback(
    (msg: WSMessage): void => {
      if (msg.type !== 'task_updated') return;
      // Ignore rows for other projects sharing the bus.
      if (projectId !== null && msg.payload.projectId !== projectId) return;
      setTasks((prev) => applyBoardTaskMessage(prev, msg));
    },
    [projectId],
  );
  useLiveUpdates(onMessage, reload);

  const clearActionError = useCallback(() => setActionError(null), []);

  const addTask = useCallback((task: BoardTask): void => {
    setTasks((prev) => (prev.some((t) => t.id === task.id) ? prev : [task, ...prev]));
  }, []);

  const moveTask = useCallback((id: number, to: BoardColumn): void => {
    let prevColumn: BoardColumn | null = null;
    setTasks((prev) =>
      prev.map((t) => {
        if (t.id !== id) return t;
        prevColumn = t.boardColumn;
        return { ...t, boardColumn: to };
      }),
    );
    if (prevColumn === null || prevColumn === to) return; // no-op move
    const revertTo = prevColumn;
    patchBoardTask(id, { boardColumn: to })
      .then((updated) => {
        if (!aliveRef.current) return;
        setTasks((prev) => prev.map((t) => (t.id === id ? updated : t)));
      })
      .catch((e: unknown) => {
        if (!aliveRef.current) return;
        // Revert the optimistic move and surface the reason.
        setTasks((prev) => prev.map((t) => (t.id === id ? { ...t, boardColumn: revertTo } : t)));
        setActionError(e instanceof Error ? e.message : String(e));
      });
  }, []);

  const patchTask = useCallback((id: number, patch: PatchBoardTaskInput): Promise<BoardTask> => {
    return patchBoardTask(id, patch).then((updated) => {
      if (aliveRef.current) setTasks((prev) => prev.map((t) => (t.id === id ? updated : t)));
      return updated;
    });
  }, []);

  return {
    tasks,
    loading,
    error,
    actionError,
    clearActionError,
    reload,
    moveTask,
    patchTask,
    addTask,
  };
}
