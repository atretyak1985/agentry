// Minimal hand-rolled markdown renderer for the Chat tab — no dependencies.
// XSS-safe by construction: it never builds HTML strings (no
// dangerouslySetInnerHTML); every fragment becomes a React text node, which
// React escapes. Supported: paragraphs, headings (#–####), fenced code
// blocks, unordered/ordered lists, **bold**, *italic*, `inline code`.

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
