// The one list of docs pages: the Markdown docs plus the hand-written docs
// pages, each with its link, title, one-line description and position.
// The docs sidebar (layouts/Docs.astro) and the docs index
// (pages/docs/index.astro) both render it, so they cannot drift. A new
// hand-written docs page joins the docs by being added to `extra` here.
import { getCollection } from 'astro:content';

export type DocsPage = { href: string; title: string; description?: string; order: number };

// Pages that are not Markdown docs, placed among them by order. The order
// is the Studio design's DocsSidebar: Get started, Signing in, Importing a
// game, Rewind, Coaching workflows, Example game, Running it on
// your desktop, What's new, For developers. (The refresh steps are a
// section of Get started, reached from its On this page rail.)
const extra: DocsPage[] = [
  {
    href: '/docs/connect',
    title: 'Get started',
    description: 'Connect the hosted tcgreplay playmat to claude.ai, ChatGPT or Claude Code, and check it works.',
    order: 10,
  },
  {
    href: '/docs/workflows',
    title: 'Coaching workflows',
    description: 'Every coaching workflow the tcgreplay playmat offers, with example prompts.',
    order: 50,
  },
  {
    href: '/docs/example',
    title: 'Example game',
    description: 'A complete Pokémon TCG Live game and its decklist to copy, so you can try tcgreplay without a game of your own.',
    order: 70,
  },
  {
    href: '/docs/whats-new',
    title: "What's new",
    description: "What changed in each tcgreplay update, and whether you need to refresh your assistant's connection.",
    order: 85,
  },
];

// docsPages returns every docs page, lowest order first, then by title.
export async function docsPages(): Promise<DocsPage[]> {
  const docs = (await getCollection('docs')).map((d) => ({
    href: `/docs/${d.id}`,
    title: d.data.title,
    description: d.data.description,
    order: d.data.order,
  }));
  return [...docs, ...extra].sort((a, b) => a.order - b.order || a.title.localeCompare(b.title));
}
