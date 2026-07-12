// Docs screen (Redesign): left DOCUMENTATION list (title + FILE.md subline,
// active item amber-tinted), right pane rendering the doc's markdown with a
// "swarmery/docs/<FILE>" mono subline. Routes: /docs (first doc) and
// /docs/{slug}. The markdown body's own leading H1 is stripped — the pane
// title comes from the doc meta.

import { useEffect, useMemo, useState } from 'react';
import { Link, useParams } from 'react-router-dom';
import type { DocDetail, DocMeta } from '../api/types';
import { fetchDoc, fetchDocs } from '../api';
import { Markdown } from '../lib/markdown';
import { Empty, ErrorBox, Loading, SectionTitle } from '../components/ui';

/** Drop a leading `# Title` line — the pane renders its own heading. */
function stripLeadingH1(markdown: string): { title: string | null; body: string } {
  const lines = markdown.split('\n');
  let i = 0;
  while (i < lines.length && (lines[i] ?? '').trim() === '') i += 1;
  const m = /^#\s+(.*)$/.exec(lines[i] ?? '');
  if (m === null) return { title: null, body: markdown };
  return { title: m[1] ?? null, body: lines.slice(i + 1).join('\n') };
}

export function Docs(): JSX.Element {
  const { slug } = useParams<{ slug: string }>();
  const [docs, setDocs] = useState<DocMeta[] | null>(null);
  const [listError, setListError] = useState<string | null>(null);
  const [doc, setDoc] = useState<DocDetail | null>(null);
  const [docError, setDocError] = useState<string | null>(null);

  useEffect(() => {
    fetchDocs()
      .then((list) => {
        setDocs(list);
        setListError(null);
      })
      .catch((e: unknown) => setListError(String(e)));
  }, []);

  const activeSlug = slug ?? docs?.[0]?.slug ?? null;

  useEffect(() => {
    if (activeSlug === null) return;
    setDoc(null);
    setDocError(null);
    fetchDoc(activeSlug)
      .then(setDoc)
      .catch((e: unknown) => setDocError(String(e)));
  }, [activeSlug]);

  const rendered = useMemo(() => (doc === null ? null : stripLeadingH1(doc.markdown)), [doc]);

  if (listError !== null) return <ErrorBox message={listError} />;
  if (docs === null) return <Loading label="docs…" />;
  if (docs.length === 0) return <Empty>no docs published by the daemon</Empty>;

  return (
    <div className="wide:grid wide:grid-cols-[230px_minmax(0,1fr)] wide:items-start wide:gap-6">
      <div className="min-w-0 wide:sticky wide:top-[76px]">
        <SectionTitle>Documentation</SectionTitle>
        <div className="overflow-hidden rounded-xl border border-line bg-surface">
          {docs.map((d) => {
            const active = d.slug === activeSlug;
            return (
              <Link
                key={d.slug}
                to={`/docs/${d.slug}`}
                aria-current={active ? 'page' : undefined}
                className={`block border-b border-line-soft px-3.5 py-2.5 transition-colors last:border-b-0 ${
                  active ? 'bg-surface2' : 'hover:bg-surface2/50'
                }`}
              >
                <span
                  className={`block text-[13px] font-semibold ${active ? 'text-brand' : 'text-ink'}`}
                >
                  {d.title}
                </span>
                <span className="mt-0.5 block font-mono text-[10.5px] text-ink-dim">{d.file}</span>
              </Link>
            );
          })}
        </div>
      </div>

      <div className="mt-5 min-w-0 rounded-xl border border-line bg-surface px-4 py-4 desk:px-7 desk:py-6 wide:mt-0">
        {docError !== null && <ErrorBox message={docError} />}
        {doc === null && docError === null && <Loading label="doc…" />}
        {doc !== null && rendered !== null && (
          <>
            <h1 className="mb-1 font-display text-[20px] leading-tight font-bold">
              {rendered.title ?? doc.title}
            </h1>
            <div className="mb-5 font-mono text-[10.5px] text-ink-dim">
              swarmery/docs/{doc.file}
            </div>
            <div className="text-[13.5px] leading-[1.65] text-ink-2">
              <Markdown text={rendered.body} />
            </div>
          </>
        )}
      </div>
    </div>
  );
}
