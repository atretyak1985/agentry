// Plans tab placeholder (/p/:slug/plans — fusion phase 4): the IA slot for the
// workspace Plans/Epics surface that Phase 10 fills (plan-doc rollups + epic
// activation). Phase 4 only reserves the route + sidebar entry so later phases
// hang their surface here without a routing change.

import { useProjectWorkspace } from '../workspace/ProjectContext';
import { Empty } from '../components/ui';

export function PlansPlaceholder(): JSX.Element {
  const { project } = useProjectWorkspace();
  return (
    <div className="px-4 pt-6 pb-10 desk:px-8">
      <Empty>
        plans for {project?.name ?? project?.slug ?? 'this project'} land here in a later phase (epics rollup +
        activation)
      </Empty>
    </div>
  );
}
