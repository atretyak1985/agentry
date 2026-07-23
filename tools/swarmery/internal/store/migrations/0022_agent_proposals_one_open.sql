-- Serialize proposal generation: at most one OPEN (proposed|approved) proposal
-- per agent, enforced by the schema so two concurrent Generate calls can't both
-- pass the code-level OpenProposalID check (TOCTOU race). internal/improve
-- inserts a placeholder 'proposed' row before running the model, so the unique
-- constraint fires immediately on the second caller.
CREATE UNIQUE INDEX idx_agent_proposals_one_open
  ON agent_change_proposals(agent)
  WHERE status IN ('proposed', 'approved');
