// What's new: the dated list of player-visible changes in
// src/data/whats-new.yaml, read and checked while the site builds. A
// missing or malformed entry fails the build with a message naming the
// entry and the field. Only pages/docs/whats-new.astro renders it.
//
// The path is relative to the site project (process.cwd() during astro
// build and the browser suite), never import.meta.url, which points into
// Astro's prerender bundle at build time.
import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import { parse } from 'yaml';

/** The list of player-visible changes, newest first. */
export const whatsNewPath = join(process.cwd(), 'src', 'data', 'whats-new.yaml');

/** One release that changed what players see. */
export type WhatsNewEntry = {
  /** The day it shipped, YYYY-MM-DD. */
  date: string;
  title: string;
  /** Whether players need to refresh their assistant's connection. */
  refresh: boolean;
  /** One short player-facing line per change; [text](href) links allowed. */
  changes: string[];
};

class WhatsNewError extends Error {
  constructor(message: string) {
    super(`what's new: ${message}`);
    this.name = 'WhatsNewError';
  }
}

// isDay reports whether s is a real calendar day written YYYY-MM-DD.
function isDay(s: string): boolean {
  if (!/^\d{4}-\d{2}-\d{2}$/.test(s)) return false;
  const d = new Date(`${s}T00:00:00Z`);
  return !Number.isNaN(d.getTime()) && d.toISOString().slice(0, 10) === s;
}

/**
 * loadWhatsNew reads the What's new list (YAML text) and returns its
 * entries newest first. It throws if the list is empty or an entry's date,
 * title, refresh answer or changes are missing or malformed.
 */
export function loadWhatsNew(yamlText: string, source = 'site/src/data/whats-new.yaml'): WhatsNewEntry[] {
  let doc: unknown;
  try {
    doc = parse(yamlText);
  } catch (err) {
    throw new WhatsNewError(`${source} is not valid YAML: ${(err as Error).message}`);
  }
  if (!Array.isArray(doc) || doc.length === 0) {
    throw new WhatsNewError(`${source} must be a list with at least one entry`);
  }
  const entries = doc.map((raw: unknown, i: number): WhatsNewEntry => {
    const e = (raw ?? {}) as Record<string, unknown>;
    // The yaml package's default (core) schema keeps unquoted dates as
    // strings; accept a Date too, in case the schema ever changes.
    const date = e.date instanceof Date ? e.date.toISOString().slice(0, 10) : e.date;
    const label = typeof e.title === 'string' && e.title.trim() ? e.title : typeof date === 'string' ? date : '';
    const where = `${source} entry ${i + 1}${label ? ` ("${label}")` : ''}`;
    if (typeof raw !== 'object' || raw === null || Array.isArray(raw)) {
      throw new WhatsNewError(`${where} must be a mapping with date, title, refresh and changes`);
    }
    if (date === undefined || date === null) throw new WhatsNewError(`${where} has no date; add one as YYYY-MM-DD`);
    if (typeof date !== 'string' || !isDay(date)) {
      throw new WhatsNewError(`${where} has date "${String(date)}", which is not a real day written YYYY-MM-DD`);
    }
    if (typeof e.title !== 'string' || !e.title.trim()) throw new WhatsNewError(`${where} has no title`);
    if (typeof e.refresh !== 'boolean') {
      throw new WhatsNewError(
        `${where} has refresh ${e.refresh === undefined ? 'missing' : `"${String(e.refresh)}"`}; it must be true or false`,
      );
    }
    if (!Array.isArray(e.changes) || e.changes.length === 0) {
      throw new WhatsNewError(`${where} has no changes; list at least one`);
    }
    e.changes.forEach((c, j) => {
      if (typeof c !== 'string' || !c.trim()) throw new WhatsNewError(`${where} has an empty change (change ${j + 1})`);
    });
    return { date, title: e.title, refresh: e.refresh, changes: e.changes as string[] };
  });
  // Newest first; entries on the same day keep their order.
  return entries.map((e, i) => ({ e, i })).sort((a, b) => b.e.date.localeCompare(a.e.date) || a.i - b.i).map(({ e }) => e);
}

const escapes: Record<string, string> = { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' };
const escape = (s: string) => s.replace(/[&<>"']/g, (c) => escapes[c]);

/**
 * changeHTML turns one change line into HTML: the text is escaped, and
 * [text](href) becomes a link when href is a site path (/...) or an
 * https:// address. Other links are left as plain text.
 */
export function changeHTML(s: string): string {
  return escape(s).replace(/\[([^\]]+)\]\(([^)\s]+)\)/g, (whole, text: string, href: string) =>
    href.startsWith('/') || href.startsWith('https://') ? `<a href="${href}">${text}</a>` : whole,
  );
}

/** The project's What's new list, newest first. */
export const whatsNew: WhatsNewEntry[] = loadWhatsNew(readFileSync(whatsNewPath, 'utf8'));
