---
name: review-heavy
description: Implement, then adversarially self-review the diff before handing to verification. Verification runs strict.
verify: strict
---
## Stage: implement
{task_prompt}

## Stage: self-review
Adversarially review the work just completed on this branch (diff vs {start_point}).
Judge on: completeness against the original contract, no collateral changes outside the declared file scope, and honest commit messages that describe what actually changed.
Look specifically for: partial implementations dressed up as complete, tests that assert nothing, edits that drifted beyond the task, and any place the first pass took a shortcut.
Fix everything you find, committing the corrections with the same Swarm-Task-Id trailer.
If, after a genuine review, nothing needs fixing, end your reply with exactly: NO-OP: review clean.
