// Minimal hand-rolled markdown renderer (Chat tab + Docs screen) — no
// dependencies. XSS-safe by construction: it never builds HTML strings (no
// dangerouslySetInnerHTML); every fragment becomes a React text node, which
// React escapes. Supported: paragraphs, headings (#–####), fenced code
// blocks, unordered/ordered lists, pipe tables, **bold**, *italic*,
// `inline code`.

import type { ReactNode } from 'react';

/* ----- inline: `code` | **bold** | *italic* ----- */

function renderInline(text: string, keyBase: string): ReactNode[] {
  // Fresh regex per call: a shared module-level /g regex would have its
  // lastIndex clobbered by the recursive bold/italic calls below.
  const inline = /`([^`]+)`|\*\*([^*]+)\*\*|\*([^*\n]+)\*/g;
  const out: ReactNode[] = [];
  let last = 0;
  let i = 0;
  for (let m = inline.exec(text); m !== null; m = inline.exec(text)) {
    if (m.index > last) out.push(text.slice(last, m.index));
    const key = `${keyBase}-${String(i)}`;
    const [, code, bold, italic] = m;
    if (code !== undefined) {
      out.push(
        <code key={key} className="rounded bg-surface2 px-1 py-px font-mono text-[0.88em] text-brand">
          {code}
        </code>,
      );
    } else if (bold !== undefined) {
      out.push(
        <strong key={key} className="font-semibold text-ink">
          {renderInline(bold, `${key}-b`)}
        </strong>,
      );
    } else if (italic !== undefined) {
      out.push(<em key={key}>{renderInline(italic, `${key}-i`)}</em>);
    }
    last = m.index + m[0].length;
    i += 1;
  }
  if (last < text.length) out.push(text.slice(last));
  return out;
}

/* ----- blocks ----- */

const HEADING_SIZES: Record<number, string> = {
  1: 'text-[15px]',
  2: 'text-[14px]',
  3: 'text-[13.5px]',
  4: 'text-[13px]',
};

/* ----- pipe tables — `| a | b |` header + `|---|---|` separator ----- */

function isTableRow(s: string): boolean {
  return s.trimStart().startsWith('|');
}

function isTableSeparator(s: string): boolean {
  const t = s.trim();
  return /^\|?[\s:|-]+\|?$/.test(t) && t.includes('-') && t.includes('|');
}

/** `| a | b |` → ['a', 'b'] (no escaped-pipe support — docs don't use it). */
function splitCells(row: string): string[] {
  return row
    .trim()
    .replace(/^\|/, '')
    .replace(/\|$/, '')
    .split('|')
    .map((c) => c.trim());
}

function renderTable(header: string, body: string[], key: string): ReactNode {
  return (
    <table key={key} className="my-2 w-full border-collapse first:mt-0 last:mb-0">
      <thead>
        <tr>
          {splitCells(header).map((cell, c) => (
            <th
              key={`${key}-h-${String(c)}`}
              className="border-b border-line px-2.5 py-1.5 text-left font-mono text-[10.5px] font-medium tracking-[0.06em] text-ink-dim uppercase"
            >
              {renderInline(cell, `${key}-h-${String(c)}`)}
            </th>
          ))}
        </tr>
      </thead>
      <tbody>
        {body.map((row, r) => (
          <tr key={`${key}-r-${String(r)}`}>
            {splitCells(row).map((cell, c) => (
              <td
                key={`${key}-r-${String(r)}-${String(c)}`}
                className="border-b border-line-soft px-2.5 py-[7px] align-top text-[13px] leading-normal"
              >
                {renderInline(cell, `${key}-r-${String(r)}-${String(c)}`)}
              </td>
            ))}
          </tr>
        ))}
      </tbody>
    </table>
  );
}

function flushParagraph(lines: string[], key: string, out: ReactNode[]): void {
  if (lines.length === 0) return;
  out.push(
    <p key={key} className="my-2 leading-relaxed whitespace-pre-line first:mt-0 last:mb-0">
      {renderInline(lines.join('\n'), key)}
    </p>,
  );
  lines.length = 0;
}

/** Renders markdown source as React elements (block-level walk). */
export function Markdown({ text }: { text: string }): JSX.Element {
  const lines = text.split('\n');
  const out: ReactNode[] = [];
  const para: string[] = [];
  let i = 0;

  while (i < lines.length) {
    const line = lines[i] ?? '';
    const key = `md-${String(i)}`;

    // Fenced code block.
    if (line.trimStart().startsWith('```')) {
      flushParagraph(para, `${key}-p`, out);
      const code: string[] = [];
      i += 1;
      while (i < lines.length && !(lines[i] ?? '').trimStart().startsWith('```')) {
        code.push(lines[i] ?? '');
        i += 1;
      }
      i += 1; // closing fence (or EOF)
      out.push(
        <pre
          key={key}
          className="my-2 overflow-x-auto rounded-lg border border-line bg-surface px-3 py-2.5 font-mono text-[11px] leading-relaxed text-ink-2 first:mt-0 last:mb-0"
        >
          <code>{code.join('\n')}</code>
        </pre>,
      );
      continue;
    }

    // Pipe table: header row + `|---|` separator, then body rows.
    if (isTableRow(line) && isTableSeparator(lines[i + 1] ?? '')) {
      flushParagraph(para, `${key}-p`, out);
      const header = line;
      i += 2; // header + separator
      const body: string[] = [];
      while (i < lines.length && isTableRow(lines[i] ?? '') && !isTableSeparator(lines[i] ?? '')) {
        body.push(lines[i] ?? '');
        i += 1;
      }
      out.push(renderTable(header, body, key));
      continue;
    }

    // Heading.
    const h = /^(#{1,4})\s+(.*)$/.exec(line);
    if (h !== null) {
      flushParagraph(para, `${key}-p`, out);
      const level = (h[1] ?? '#').length;
      out.push(
        <div
          key={key}
          className={`mt-3 mb-1.5 font-semibold text-ink first:mt-0 ${HEADING_SIZES[level] ?? 'text-[13px]'}`}
        >
          {renderInline(h[2] ?? '', key)}
        </div>,
      );
      i += 1;
      continue;
    }

    // List (unordered or ordered) — consecutive item lines form one list.
    const isItem = (s: string): boolean => /^\s*([-*]|\d+\.)\s+/.test(s);
    if (isItem(line)) {
      flushParagraph(para, `${key}-p`, out);
      const ordered = /^\s*\d+\.\s+/.test(line);
      const items: string[] = [];
      while (i < lines.length && isItem(lines[i] ?? '')) {
        items.push((lines[i] ?? '').replace(/^\s*([-*]|\d+\.)\s+/, ''));
        i += 1;
      }
      const rows = items.map((item, n) => (
        <li key={`${key}-li-${String(n)}`}>{renderInline(item, `${key}-li-${String(n)}`)}</li>
      ));
      out.push(
        ordered ? (
          <ol key={key} className="my-2 list-decimal space-y-1 pl-5 first:mt-0 last:mb-0">
            {rows}
          </ol>
        ) : (
          <ul key={key} className="my-2 list-disc space-y-1 pl-5 first:mt-0 last:mb-0">
            {rows}
          </ul>
        ),
      );
      continue;
    }

    // Blank line → paragraph boundary.
    if (line.trim() === '') {
      flushParagraph(para, `${key}-p`, out);
      i += 1;
      continue;
    }

    para.push(line);
    i += 1;
  }
  flushParagraph(para, 'md-tail-p', out);

  return <>{out}</>;
}
