// The docs layout's "On this page" list: the h2 headings of a rendered
// docs article, each given an id to link to. Markdown headings already
// carry ids; the hand-written pages' h2s mostly do not, so withHeadingIds
// gives each one without an id a slug of its text, unique on the page.

export type Heading = { id: string; text: string };

const h2 = /<h2\b([^>]*)>([\s\S]*?)<\/h2>/gi;
const idAttr = /\sid\s*=\s*"([^"]*)"/i;
const anyId = /\sid\s*=\s*"([^"]*)"/gi;

// decode turns the few HTML entities Astro writes back into text.
function decode(s: string): string {
  return s
    .replace(/&#x([0-9a-f]+);/gi, (_, h) => String.fromCodePoint(parseInt(h, 16)))
    .replace(/&#(\d+);/g, (_, d) => String.fromCodePoint(Number(d)))
    .replace(/&quot;/g, '"')
    .replace(/&lt;/g, '<')
    .replace(/&gt;/g, '>')
    .replace(/&nbsp;/g, ' ')
    .replace(/&amp;/g, '&');
}

// slug is a heading's text as an id: lower case, letters and digits
// joined by hyphens.
export function slug(text: string): string {
  return (
    text
      .normalize('NFKD')
      .replace(/[̀-ͯ]/g, '')
      .toLowerCase()
      .replace(/[^a-z0-9]+/g, '-')
      .replace(/^-+|-+$/g, '') || 'section'
  );
}

// withHeadingIds returns the article's HTML with an id on every h2, and
// the h2s in order with their ids and plain text.
export function withHeadingIds(html: string): { html: string; headings: Heading[] } {
  const taken = new Set([...html.matchAll(anyId)].map((m) => m[1]));
  const headings: Heading[] = [];
  const out = html.replace(h2, (whole, attrs: string, inner: string) => {
    const text = decode(inner.replace(/<[^>]*>/g, '')).replace(/\s+/g, ' ').trim();
    const have = idAttr.exec(attrs);
    if (have) {
      headings.push({ id: have[1], text });
      return whole;
    }
    let id = slug(text);
    for (let n = 2; taken.has(id); n++) id = `${slug(text)}-${n}`;
    taken.add(id);
    headings.push({ id, text });
    return `<h2 id="${id}"${attrs}>${inner}</h2>`;
  });
  return { html: out, headings };
}
