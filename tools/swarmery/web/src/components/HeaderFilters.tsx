// Header filter slot: pages teleport their filter controls into the app
// header (the `#header-filters` div App.tsx renders on filter pages) so the
// chrome adapts to the section — /sessions shows title search + status chips,
// /system shows name search + level chips. State stays in the page; only the
// rendering moves. The slot is `hidden desk:flex`, so below the desk
// breakpoint pages keep their own in-body copy of the same controls.

import { useEffect, useState } from 'react';
import type { ReactNode } from 'react';
import { createPortal } from 'react-dom';

export function HeaderFilters({ children }: { children: ReactNode }): JSX.Element | null {
  const [target, setTarget] = useState<HTMLElement | null>(null);
  // Looked up after mount — App renders the header (and the slot) before the
  // routed page, so the div exists by the time this effect runs.
  useEffect(() => {
    setTarget(document.getElementById('header-filters'));
  }, []);
  if (target === null) return null;
  return createPortal(children, target);
}
