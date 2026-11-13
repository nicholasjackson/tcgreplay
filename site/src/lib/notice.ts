// The fan-project notice, the card-data credit and the help address, read
// while the site builds from the one file the website, the board and the
// ChatGPT listing share (internal/notice/notice.json), so their wording
// can't drift apart. A missing or broken file fails the build with a
// message naming the problem.
//
// Paths are relative to the site project (process.cwd() during astro
// build and the browser suite), never import.meta.url, which points into
// Astro's prerender bundle at build time.
import { readFileSync } from 'node:fs';
import { join } from 'node:path';

/** The shared notice file, read by the website build and the board build. */
export const noticePath = join(process.cwd(), '..', 'internal', 'notice', 'notice.json');

/** A named link: a card-data source or the help channel. */
export type NoticeLink = { name: string; url: string };

/** The shared wording, as internal/notice/notice.json holds it. */
export type Notice = {
  /** The unofficial fan project notice, naming the four companies. */
  notice: string;
  /** Where card data and images come from: a lead-in and each source. */
  credit: { lead: string; sources: NoticeLink[] };
  /** The project's only help channel, its GitHub issues page. */
  help: { label: string; url: string };
};

/** The companies the notice must name. */
export const companies = ['Nintendo', 'The Pokémon Company', 'Creatures', 'Game Freak'] as const;

class NoticeError extends Error {
  constructor(message: string) {
    super(`site notice: ${message}`);
    this.name = 'NoticeError';
  }
}

/**
 * loadNotice parses the shared notice file (JSON text). It throws, naming
 * the problem, if the JSON is invalid, a field is missing or empty, the
 * notice does not name all four companies, or an address is not https.
 */
export function loadNotice(jsonText: string, source = 'internal/notice/notice.json'): Notice {
  let raw: any;
  try {
    raw = JSON.parse(jsonText);
  } catch (err) {
    throw new NoticeError(`${source} is not valid JSON: ${(err as Error).message}`);
  }
  const text = (value: unknown, field: string): string => {
    if (typeof value !== 'string' || value.trim() === '') {
      throw new NoticeError(`${source} has no ${field}; the website shows it, so the build stops`);
    }
    return value.trim();
  };
  const https = (value: unknown, field: string): string => {
    const url = text(value, field);
    let u: URL;
    try {
      u = new URL(url);
    } catch {
      throw new NoticeError(`${source} sets ${field} to "${url}", which is not a URL; it must be an https:// address`);
    }
    if (u.protocol !== 'https:' || !u.hostname) {
      throw new NoticeError(`${source} sets ${field} to "${url}"; it must be an https:// address`);
    }
    return url;
  };

  const notice = text(raw?.notice, 'notice');
  for (const c of companies) {
    if (!notice.includes(c)) {
      throw new NoticeError(`${source}: the notice does not name ${c}; it must name ${companies.join(', ')}`);
    }
  }

  const lead = text(raw?.credit?.lead, 'credit.lead');
  const list: unknown[] = Array.isArray(raw?.credit?.sources) ? raw.credit.sources : [];
  if (list.length === 0) {
    throw new NoticeError(`${source} has no credit.sources; the website credits where card data comes from, so the build stops`);
  }
  const sources = list.map((s: any, i) => ({
    name: text(s?.name, `credit.sources[${i}].name`),
    url: https(s?.url, `credit.sources[${i}].url`),
  }));

  const help = { label: text(raw?.help?.label, 'help.label'), url: https(raw?.help?.url, 'help.url') };

  return { notice, credit: { lead, sources }, help };
}

function readNotice(): string {
  try {
    return readFileSync(noticePath, 'utf8');
  } catch (err) {
    throw new NoticeError(`cannot read ${noticePath}: ${(err as Error).message}`);
  }
}

/** The shared wording, read once when the site builds. */
export const notice: Notice = loadNotice(readNotice());
