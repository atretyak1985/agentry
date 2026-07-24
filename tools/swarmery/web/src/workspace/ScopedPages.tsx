// Scoped tool-page wrappers (fusion phase 4): render the existing fleet tool
// pages (Serena / Graphify / Architecture) pinned to the current workspace
// project via their `scopedSlug` prop, so they show that one project without
// their in-page dropdown. The pages are NOT forked — this only passes the slug.
// Sessions / Analytics / Retro need no wrapper: they read the global scope,
// which ProjectContext already drives from the :slug.

import { Architecture } from '../pages/Architecture';
import { Graphify } from '../pages/Graphify';
import { Serena } from '../pages/Serena';
import { useProjectWorkspace } from './ProjectContext';

export function ScopedSerena(): JSX.Element {
  const { slug } = useProjectWorkspace();
  return <Serena scopedSlug={slug} />;
}

export function ScopedGraphify(): JSX.Element {
  const { slug } = useProjectWorkspace();
  return <Graphify scopedSlug={slug} />;
}

export function ScopedArchitecture(): JSX.Element {
  const { slug } = useProjectWorkspace();
  return <Architecture scopedSlug={slug} />;
}
