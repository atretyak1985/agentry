import { projectColor } from '../lib/colors';
import { projectLabel } from '../lib/format';

/**
 * Project name rendered in the project's stable accent color (hashed by slug,
 * same palette as the accent dots in `lib/colors`). One place owns the
 * "project name → color" mapping so every surface — session lists, detail
 * header, spine, tables — shows a given project in the same hue.
 *
 * `className` carries typography/layout (font, size, truncate); the color is
 * applied inline and wins over any text-color utility.
 */
export function ProjectName({
  name,
  slug,
  className,
}: {
  name: string | null | undefined;
  slug: string;
  className?: string;
}): JSX.Element {
  return (
    <span className={className} style={{ color: projectColor(slug) }}>
      {projectLabel(name, slug)}
    </span>
  );
}
