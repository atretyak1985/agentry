// Stable per-project accent dots (Redesign rails/rows): a small palette
// hashed by slug so a project keeps its color across screens and reloads.

// No red in the palette — a red dot would read as "failing project".
const PROJECT_PALETTE = [
  '#f5b84a', // amber
  '#6fb4f0', // blue
  '#4ade9c', // green
  '#c893e8', // purple (Redesign fourth-project tone)
] as const;

export function projectColor(slug: string): string {
  let hash = 0;
  for (let i = 0; i < slug.length; i += 1) {
    hash = (hash * 31 + slug.charCodeAt(i)) >>> 0;
  }
  return PROJECT_PALETTE[hash % PROJECT_PALETTE.length] ?? '#7c8da3';
}
