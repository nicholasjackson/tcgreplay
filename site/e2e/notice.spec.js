// The shared fan-project notice, card-data credit and help address
// (internal/notice/notice.json), which the website, the board and the
// ChatGPT listing all read. A missing or broken file must stop the build
// with a message naming the problem; every page's build reads it through
// the shared layout. No browser page is needed.
import { readFileSync } from 'node:fs';
import { test, expect } from '@playwright/test';
import { loadNotice, notice } from '../src/lib/notice.ts';

const noticeJSON = () => readFileSync(new URL('../../internal/notice/notice.json', import.meta.url), 'utf8');

// The file's own content, parsed without the site's loader. The wording
// lives only in that file, so these tests check its shape and rules (the
// four companies, the two sources, https addresses), never a copy of it.
const shared = () => JSON.parse(noticeJSON());
const companies = ['Nintendo', 'The Pokémon Company', 'Creatures', 'Game Freak'];

// fixture is the shared file's JSON with edit applied to a copy of it.
function fixture(edit = () => {}) {
  const v = shared();
  edit(v);
  return JSON.stringify(v);
}

test('the real shared file gives the notice, the credit and the help address', () => {
  const want = shared();
  expect(loadNotice(noticeJSON())).toEqual(want);
  expect(notice).toEqual(want);
  expect(notice.notice).toContain('unofficial fan project');
  for (const c of companies) expect(notice.notice, `notice names ${c}`).toContain(c);
  expect(notice.credit.sources.map((s) => s.name)).toEqual(['TCGdex', 'malie.io']);
  for (const s of notice.credit.sources) expect(new URL(s.url).protocol, `${s.name} address`).toBe('https:');
  // The help channel is the project's GitHub issues page.
  const help = new URL(notice.help.url);
  expect(help.protocol).toBe('https:');
  expect(help.host).toBe('github.com');
  expect(help.pathname.endsWith('/issues')).toBe(true);
});

test('a well-formed file loads', () => {
  expect(loadNotice(fixture())).toEqual(shared());
});

test('a file that is not JSON fails, naming the problem', () => {
  expect(() => loadNotice('{ "notice": ')).toThrow(/site notice: .*not valid JSON/);
});

for (const [field, edit] of [
  ['notice', (v) => delete v.notice],
  ['credit.lead', (v) => (v.credit.lead = '  ')],
  ['help.label', (v) => delete v.help.label],
  ['help.url', (v) => delete v.help.url],
  ['credit.sources[1].name', (v) => (v.credit.sources[1].name = '')],
]) {
  test(`a file without ${field} fails with a message naming it`, () => {
    expect(() => loadNotice(fixture(edit))).toThrow(`has no ${field}`);
  });
}

test('a file with no credit sources fails with a message naming them', () => {
  expect(() => loadNotice(fixture((v) => (v.credit.sources = [])))).toThrow('has no credit.sources');
  expect(() => loadNotice(fixture((v) => delete v.credit))).toThrow(/has no credit/);
});

for (const c of companies) {
  test(`a notice that drops ${c} fails with a message naming it`, () => {
    const json = fixture((v) => (v.notice = v.notice.replace(c, 'someone')));
    expect(() => loadNotice(json)).toThrow(`does not name ${c}`);
  });
}

for (const [field, edit] of [
  ['credit.sources[0].url', (v) => (v.credit.sources[0].url = 'http://example.org')],
  ['credit.sources[1].url', (v) => (v.credit.sources[1].url = 'example.org')],
  ['help.url', (v) => (v.help.url = 'http://example.org/issues')],
]) {
  test(`a non-https ${field} fails with a message naming it`, () => {
    expect(() => loadNotice(fixture(edit))).toThrow(new RegExp(`${field.replace(/[[\].]/g, '\\$&')} to .*https://`));
  });
}
