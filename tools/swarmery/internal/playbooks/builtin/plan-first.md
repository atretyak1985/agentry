---
name: plan-first
description: Produce a short step plan first, then implement it in a second stage. Verification runs at the normal bar.
verify: normal
---
## Stage: plan
Do NOT edit any files in this stage. Read the codebase as needed and produce a concise, ordered implementation plan for the task below — the concrete steps, the files each step touches, and the risks you foresee.
Your entire reply IS the plan; it is handed verbatim to the next stage, so make it self-contained and unambiguous. Do not commit anything.

{task_prompt}

## Stage: implement
Implement the task by following the plan produced in the previous stage, reproduced here:

{previous_stage_output}

Work through the plan step by step. If a step turns out to be wrong or impossible, correct course and note it in your commit message rather than silently skipping it. Commit your work with the Swarm-Task-Id trailer.
