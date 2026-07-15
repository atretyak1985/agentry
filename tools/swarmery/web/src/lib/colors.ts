// Stable per-project accent dots (Redesign rails/rows): a small palette
// hashed by slug so a project keeps its color across screens and reloads.

// Reserved hues never enter the palette, so a project color never collides
// with a status meaning: no red (reads as "failing"), no green (reads as
// "active/working"), and no gray/white (reserved for neutral/done text).
const PROJECT_PALETTE = [
  '#e8a13a', // amber
  '#6fb4f0', // blue
  '#c58be0', // purple (Canvas fourth-project tone)
  '#e88ab0', // pink (replaces green — green is the "working" status color)
] as const;

export function projectColor(slug: string): string {
  let hash = 0;
  for (let i = 0; i < slug.length; i += 1) {
    hash = (hash * 31 + slug.charCodeAt(i)) >>> 0;
  }
  return PROJECT_PALETTE[hash % PROJECT_PALETTE.length] ?? '#8b8f99';
}
