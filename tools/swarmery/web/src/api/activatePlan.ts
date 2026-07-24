// Activate a plan into board tasks (fusion phase 8 — shared helper).
//
// Scope note: the full plan → one-task-per-phase-doc fan-out belongs to Phase 10
// (Plans/Epics), which will read the phase docs off disk. The workspace schema
// today does NOT expose per-phase-doc paths through the tasks API (0006 omits
// task_phases/task_artifacts for phase docs), so Phase 8 activation creates a
// SINGLE Triage board task seeded from the plan: the operator promotes it and
// the dispatcher picks it up. When Phase 10 lands it replaces this helper with
// the per-phase fan-out (same POST /api/board/tasks path). Kept in a dedicated
// module so that swap is a one-file change.

import { createBoardTask, type CreateBoardTaskInput } from '../api';
import type { BoardTask, TaskSummary } from '../api/types';

/** What the Planning "done" state knows about a freshly-detected plan. */
export interface PlanRef {
  /** The workspace task row wsingest created for the plan dir. */
  task: TaskSummary;
  /** The project the plan belongs to (board tasks need a numeric project id). */
  projectId: number;
  /** The originating idea (folded into the task prompt for context). */
  idea: string;
}

/**
 * Create one Triage board task from a detected plan and return it. The task
 * prompt points the executor at the plan (the workspace task-id / plan dir) plus
 * the originating idea, so a dispatched run has the full plan to work from. The
 * title reuses the plan's own title.
 */
export async function activatePlan(ref: PlanRef): Promise<BoardTask> {
  const title = ref.task.title.trim() === '' ? 'Execute plan' : ref.task.title.trim();
  const prompt = [
    `Execute the plan tracked as workspace task ${ref.task.externalId}.`,
    '',
    'Read the plan directory (plan/README.md + the phase/step docs) in the private',
    'workspace for this project and carry it out phase by phase, following each',
    "doc's acceptance criteria. The plan was generated from this idea:",
    '',
    ref.idea.trim(),
  ].join('\n');

  const input: CreateBoardTaskInput = {
    projectId: ref.projectId,
    title,
    prompt,
    boardColumn: 'triage',
  };
  return createBoardTask(input);
}
