// Theme provider (Fusion phase 16): mode (light/dark/system) + curated palette.
//
// Persists `{mode, palette}` as JSON to localStorage `swarmery.theme` and applies
// three attributes on <html>:
//   data-mode    — the RESOLVED mode (system → light|dark), the CSS source of
//                  truth for the light overrides in index.css.
//   data-theme   — mirror of data-mode, kept for any legacy/external reader.
//   data-palette — the active palette id (index.css `[data-palette]` blocks).
//
// `system` follows `prefers-color-scheme` LIVE via a matchMedia listener. The
// pre-paint snippet in index.html applies the same attributes before first paint
// (no flash); this provider re-reads the identical stored state on init so
// hydration never flips the theme. localStorage is guarded (private mode leaves
// storage throwing — the in-memory choice still applies for the session), the
// same try/catch idiom as scope.tsx.

import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
  type ReactNode,
} from 'react';
import {
  DEFAULT_MODE,
  DEFAULT_PALETTE,
  isMode,
  isPaletteId,
  type Mode,
  type PaletteId,
  type ResolvedMode,
} from '../theme/palettes';

/** Back-compat alias: `theme` used to be the resolved light|dark mode; existing
 *  consumers (App toggle, Analytics chart tokens, WorkspaceShell) still read it. */
export type Theme = ResolvedMode;

const STORAGE_KEY = 'swarmery.theme';

interface ThemeChoice {
  mode: Mode;
  palette: PaletteId;
}

interface ThemeValue {
  /** User's mode choice (may be `system`). */
  mode: Mode;
  /** Active palette id. */
  palette: PaletteId;
  /** Mode actually applied to the DOM (system already resolved). */
  theme: ResolvedMode;
  setMode: (next: Mode) => void;
  setPalette: (next: PaletteId) => void;
  /** Legacy light↔dark flip. On `system` it snaps to the opposite of the
   *  currently-resolved mode (leaving `system` for an explicit choice). */
  toggle: () => void;
}

const ThemeContext = createContext<ThemeValue>({
  mode: DEFAULT_MODE,
  palette: DEFAULT_PALETTE,
  theme: 'dark',
  setMode: () => undefined,
  setPalette: () => undefined,
  toggle: () => undefined,
});

function readStored(): ThemeChoice {
  try {
    const raw = window.localStorage.getItem(STORAGE_KEY);
    if (raw === null) return { mode: DEFAULT_MODE, palette: DEFAULT_PALETTE };
    // Legacy value: a bare 'light' | 'dark' string (pre-phase-16).
    if (raw === 'light' || raw === 'dark') return { mode: raw, palette: DEFAULT_PALETTE };
    const parsed: unknown = JSON.parse(raw);
    if (typeof parsed !== 'object' || parsed === null) {
      return { mode: DEFAULT_MODE, palette: DEFAULT_PALETTE };
    }
    const rec = parsed as Record<string, unknown>;
    return {
      mode: isMode(rec.mode) ? rec.mode : DEFAULT_MODE,
      palette: isPaletteId(rec.palette) ? rec.palette : DEFAULT_PALETTE,
    };
  } catch {
    return { mode: DEFAULT_MODE, palette: DEFAULT_PALETTE }; // storage/JSON disabled
  }
}

function persist(choice: ThemeChoice): void {
  try {
    window.localStorage.setItem(STORAGE_KEY, JSON.stringify(choice));
  } catch {
    // storage disabled — the in-memory choice still applies this session
  }
}

function systemPrefersDark(): boolean {
  try {
    return window.matchMedia('(prefers-color-scheme: dark)').matches;
  } catch {
    return true; // matchMedia unavailable → dark stays the app default
  }
}

function resolveMode(mode: Mode, systemDark: boolean): ResolvedMode {
  if (mode === 'system') return systemDark ? 'dark' : 'light';
  return mode;
}

function applyAttributes(resolved: ResolvedMode, palette: PaletteId): void {
  const root = document.documentElement;
  root.dataset.mode = resolved;
  root.dataset.theme = resolved; // legacy mirror
  root.dataset.palette = palette;
  // The pre-paint snippet set an inline hex background on <html> to avoid a
  // flash before the stylesheet loaded. Now that index.css is present, hand the
  // html background back to the live token so a runtime theme switch never
  // leaves a stale color peeking behind the shell / scroll overflow.
  root.style.background = 'var(--color-bg)';
}

export function ThemeProvider({ children }: { children: ReactNode }): JSX.Element {
  const initial = readStored();
  const [mode, setModeState] = useState<Mode>(initial.mode);
  const [palette, setPaletteState] = useState<PaletteId>(initial.palette);
  const [systemDark, setSystemDark] = useState<boolean>(systemPrefersDark);

  const resolved = resolveMode(mode, systemDark);

  // Track the OS preference live so `system` follows a theme switch without a
  // reload. The listener stays mounted regardless of mode (cheap) so switching
  // TO system picks up the current value immediately.
  useEffect(() => {
    let mql: MediaQueryList;
    try {
      mql = window.matchMedia('(prefers-color-scheme: dark)');
    } catch {
      return undefined; // matchMedia unavailable — systemDark stays at default
    }
    const onChange = (e: MediaQueryListEvent): void => setSystemDark(e.matches);
    mql.addEventListener('change', onChange);
    return () => mql.removeEventListener('change', onChange);
  }, []);

  // Apply attributes on mount and whenever the resolved mode or palette changes.
  useEffect(() => {
    applyAttributes(resolved, palette);
  }, [resolved, palette]);

  const setMode = useCallback(
    (next: Mode): void => {
      setModeState(next);
      persist({ mode: next, palette });
    },
    [palette],
  );

  const setPalette = useCallback(
    (next: PaletteId): void => {
      setPaletteState(next);
      persist({ mode, palette: next });
    },
    [mode],
  );

  const toggle = useCallback((): void => {
    setMode(resolved === 'light' ? 'dark' : 'light');
  }, [resolved, setMode]);

  const value = useMemo(
    () => ({ mode, palette, theme: resolved, setMode, setPalette, toggle }),
    [mode, palette, resolved, setMode, setPalette, toggle],
  );
  return <ThemeContext.Provider value={value}>{children}</ThemeContext.Provider>;
}

export function useTheme(): ThemeValue {
  return useContext(ThemeContext);
}
