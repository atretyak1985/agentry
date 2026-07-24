// Curated palette catalog (Fusion phase 16) — the single source of truth for
// the 6 palettes shipped. The CSS token blocks live in index.css keyed by
// `[data-palette='<id>']`; this module carries the id/label/description plus the
// four preview swatches the picker renders (resolved hex, kept in sync with the
// dark-mode accent/brand values in index.css). Deliberately 6, not 70 — every
// palette must look intentional on every surface (DESIGN.md §2.5).

export type Mode = 'light' | 'dark' | 'system';
/** The mode actually applied to the DOM once `system` is resolved. */
export type ResolvedMode = 'light' | 'dark';
export type PaletteId = 'swarm' | 'ocean' | 'forest' | 'sunset' | 'zen' | 'ember';

export interface Palette {
  id: PaletteId;
  label: string;
  /** One-line character note shown under the label in the picker. */
  hint: string;
  /** 4 preview swatches [bg, surface, brand, secondary-accent] — dark-mode
   *  values, matching the `[data-palette]` block so the tile reads true. */
  swatches: readonly [string, string, string, string];
}

export const PALETTES: readonly Palette[] = [
  {
    id: 'swarm',
    label: 'Swarm',
    hint: 'warm amber — the original',
    swatches: ['#0e0f12', '#131519', '#e8a13a', '#58c08a'],
  },
  {
    id: 'ocean',
    label: 'Ocean',
    hint: 'cool blue, deep-sea',
    swatches: ['#0b0f14', '#101720', '#4fb0e8', '#48c9a0'],
  },
  {
    id: 'forest',
    label: 'Forest',
    hint: 'muted green canopy',
    swatches: ['#0c1210', '#101a16', '#5fc98a', '#d8b24a'],
  },
  {
    id: 'sunset',
    label: 'Sunset',
    hint: 'warm coral & magenta',
    swatches: ['#140d0e', '#1c1314', '#f08a5a', '#e08ab0'],
  },
  {
    id: 'zen',
    label: 'Zen',
    hint: 'neutral cool gray',
    swatches: ['#101214', '#16191c', '#8fa3b8', '#7fb0e0'],
  },
  {
    id: 'ember',
    label: 'Ember',
    hint: 'deep forge orange',
    swatches: ['#120e0c', '#1a1512', '#e87a4a', '#e8a13a'],
  },
] as const;

const PALETTE_IDS = new Set<string>(PALETTES.map((p) => p.id));

export const DEFAULT_PALETTE: PaletteId = 'swarm';
export const DEFAULT_MODE: Mode = 'system';

export function isPaletteId(v: unknown): v is PaletteId {
  return typeof v === 'string' && PALETTE_IDS.has(v);
}

export function isMode(v: unknown): v is Mode {
  return v === 'light' || v === 'dark' || v === 'system';
}
