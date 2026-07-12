// Hand-rolled inline-SVG sparkline for the Overview KPI tiles (Redesign):
// a single polyline over a 100×24 viewBox stretched to the tile width, with
// a highlight dot on the selected day. No chart deps.

const W = 100;
const H = 24;

/** Redesign stroke/dot tones per tile. */
const TONES = {
  dim: { stroke: '#7c8da3', opacity: 0.7, dot: '#dce6f2' },
  amber: { stroke: '#f5b84a', opacity: 0.55, dot: '#f5b84a' },
  red: { stroke: '#f0716f', opacity: 0.6, dot: '#f0716f' },
} as const;

export function Sparkline({
  values,
  highlight,
  tone = 'dim',
}: {
  /** Series values, oldest → newest. */
  values: number[];
  /** Index of the selected day (highlight dot). */
  highlight: number;
  tone?: keyof typeof TONES;
}): JSX.Element | null {
  if (values.length < 2) return null;

  const min = Math.min(...values);
  const max = Math.max(...values);
  const x = (i: number): number => (i * W) / (values.length - 1);
  const y = (v: number): number => (max === min ? H / 2 : 21 - ((v - min) / (max - min)) * 18);
  const points = values.map((v, i) => `${x(i).toFixed(1)},${y(v).toFixed(1)}`).join(' ');

  const hi = Math.min(Math.max(highlight, 0), values.length - 1);
  const hiValue = values[hi] ?? 0;
  const { stroke, opacity, dot } = TONES[tone];

  return (
    <svg
      viewBox={`0 0 ${String(W)} ${String(H)}`}
      preserveAspectRatio="none"
      className="mt-2.5 block h-6 w-full"
      aria-hidden="true"
    >
      <polyline points={points} fill="none" stroke={stroke} strokeWidth="1.5" opacity={opacity} />
      <circle cx={x(hi).toFixed(1)} cy={y(hiValue).toFixed(1)} r="2.5" fill={dot} />
    </svg>
  );
}
