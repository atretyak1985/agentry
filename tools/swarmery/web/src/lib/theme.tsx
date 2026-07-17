// Theme provider (light/dark): persists to localStorage, falls back to the
// OS preference, and applies document.documentElement.dataset.theme so the
// index.css `:root[data-theme='light']` token override cascades everywhere.
// Mirrors the localStorage try/catch guard pattern in scope.tsx (private
// mode disables storage but the in-memory theme still applies this session).

import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
  type ReactNode,
} from 'react';

export type Theme = 'light' | 'dark';

const STORAGE_KEY = 'swarmery.theme';

interface ThemeValue {
  theme: Theme;
  setTheme: (next: Theme) => void;
  toggle: () => void;
}

const ThemeContext = createContext<ThemeValue>({
  theme: 'dark',
  setTheme: () => undefined,
  toggle: () => undefined,
});

function storedTheme(): Theme | null {
  try {
    const v = window.localStorage.getItem(STORAGE_KEY);
    return v === 'light' || v === 'dark' ? v : null;
  } catch {
    return null; // storage disabled (private mode) → fall through to OS pref
  }
}

function preferredTheme(): Theme {
  const stored = storedTheme();
  if (stored !== null) return stored;
  try {
    return window.matchMedia('(prefers-color-scheme: light)').matches ? 'light' : 'dark';
  } catch {
    return 'dark'; // matchMedia unavailable → dark stays the app default
  }
}

function applyTheme(theme: Theme): void {
  document.documentElement.dataset.theme = theme;
}

export function ThemeProvider({ children }: { children: ReactNode }): JSX.Element {
  const [theme, setThemeState] = useState<Theme>(preferredTheme);

  // Apply on mount and whenever the theme changes.
  useEffect(() => {
    applyTheme(theme);
  }, [theme]);

  const setTheme = useCallback((next: Theme): void => {
    setThemeState(next);
    try {
      window.localStorage.setItem(STORAGE_KEY, next);
    } catch {
      // storage disabled — the in-memory theme still applies this session
    }
  }, []);

  const toggle = useCallback((): void => {
    setTheme(theme === 'light' ? 'dark' : 'light');
  }, [theme, setTheme]);

  const value = useMemo(() => ({ theme, setTheme, toggle }), [theme, setTheme, toggle]);
  return <ThemeContext.Provider value={value}>{children}</ThemeContext.Provider>;
}

export function useTheme(): ThemeValue {
  return useContext(ThemeContext);
}
