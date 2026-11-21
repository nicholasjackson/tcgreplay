// The facts the site reads from the playmat's own sources while it builds:
// the public address and the hosted limits from the App Platform
// definition. A missing or wrong fact must stop the build with a message
// naming it; every page's build reads the facts through the shared layout.
// No browser page is needed.
import { readFileSync } from 'node:fs';
import { test, expect } from './fixtures.js';
import { loadLimits, loadPublicURL, limits, site, humanDuration, parseGoDuration } from '../src/lib/facts.ts';

const appYaml = () => readFileSync(new URL('../../.do/app.yaml', import.meta.url), 'utf8');

// fixture is an App Platform definition with the playmat's public address,
// its three limits and its domains; drop leaves one env out, publicURL
// replaces the address and primary the PRIMARY domain (null leaves the
// domains out).
function fixture({ drop, publicURL = 'https://tcgreplay.app', primary = 'tcgreplay.app' } = {}) {
  const envs = [
    ['TCGREPLAY_PUBLIC_URL', publicURL],
    ['TCGREPLAY_RATE_LIMIT', '"120"'],
    ['TCGREPLAY_MAX_USERS', '"5"'],
    ['TCGREPLAY_IDLE_TIMEOUT', '"2h"'],
  ]
    .filter(([key]) => key !== drop)
    .map(([key, value]) => `  - key: ${key}\n    value: ${value}\n    scope: RUN_TIME\n`)
    .join('');
  const domains =
    primary === null ? '' : `domains:\n  - domain: ${primary}\n    type: PRIMARY\n  - domain: tcgreplay.demo.gs\n    type: ALIAS\n`;
  return `name: tcgreplay\nservices:\n  - name: tcgreplay\n    http_port: 8080\nenvs:\n${envs}${domains}`;
}

const limitKeys = ['TCGREPLAY_RATE_LIMIT', 'TCGREPLAY_MAX_USERS', 'TCGREPLAY_IDLE_TIMEOUT'];

test('the real App Platform definition gives the hosted limits', () => {
  const want = { requestsPerMinute: 300, maxUsers: 10, idleTimeout: '60 minutes' };
  expect(loadLimits(appYaml())).toEqual(want);
  expect(limits).toEqual(want);
});

test('a definition with all three limits loads them', () => {
  expect(loadLimits(fixture())).toEqual({ requestsPerMinute: 120, maxUsers: 5, idleTimeout: '2 hours' });
});

for (const key of limitKeys) {
  test(`a definition without ${key} fails with a message naming it`, () => {
    const yaml = fixture({ drop: key });
    expect(yaml).not.toContain(key);
    expect(() => loadLimits(yaml)).toThrow(key);
  });

  test(`${key} removed from the real definition fails with a message naming it`, () => {
    const text = appYaml();
    // Remove the "- key: X" entry and its indented lines.
    const re = new RegExp(`\\n\\s*- key: ${key}\\n(?: {4}\\S.*\\n)*`);
    const without = text.replace(re, '\n');
    expect(without).not.toContain(`- key: ${key}`);
    expect(() => loadLimits(without)).toThrow(new RegExp(`does not set ${key}`));
  });
}

test('a definition with an empty limit fails with a message naming it', () => {
  const yaml = fixture().replace('value: "5"', 'value: ""');
  expect(() => loadLimits(yaml)).toThrow('TCGREPLAY_MAX_USERS');
});

test('a definition with the settings on the service, not the app, fails', () => {
  const onService =
    'name: tcgreplay\nservices:\n  - name: tcgreplay\n    http_port: 8080\n    envs:\n' +
    '      - key: TCGREPLAY_RATE_LIMIT\n        value: "120"\n' +
    '      - key: TCGREPLAY_MAX_USERS\n        value: "5"\n' +
    '      - key: TCGREPLAY_IDLE_TIMEOUT\n        value: "2h"\n';
  expect(() => loadLimits(onService)).toThrow(/does not set TCGREPLAY_\w+ in the app's settings/);
});

test('the real App Platform definition gives the public address and the addresses built from it', () => {
  expect(loadPublicURL(appYaml())).toBe('https://tcgreplay.app');
  expect(site.publicURL).toBe('https://tcgreplay.app');
  expect(site.mcpURL).toBe('https://tcgreplay.app/mcp');
  expect(site.boardURL).toBe('https://tcgreplay.app/board');
});

test('a definition with the public address and a matching PRIMARY domain loads it', () => {
  expect(loadPublicURL(fixture())).toBe('https://tcgreplay.app');
});

test('a definition without TCGREPLAY_PUBLIC_URL fails with a message naming it', () => {
  const yaml = fixture({ drop: 'TCGREPLAY_PUBLIC_URL' });
  expect(yaml).not.toContain('TCGREPLAY_PUBLIC_URL');
  expect(() => loadPublicURL(yaml)).toThrow(/does not set TCGREPLAY_PUBLIC_URL/);
});

test('TCGREPLAY_PUBLIC_URL removed from the real definition fails with a message naming it', () => {
  const without = appYaml().replace(/\n\s*- key: TCGREPLAY_PUBLIC_URL\n(?: {4}\S.*\n)*/, '\n');
  expect(without).not.toContain('- key: TCGREPLAY_PUBLIC_URL');
  expect(() => loadPublicURL(without)).toThrow(/does not set TCGREPLAY_PUBLIC_URL/);
});

test('a public address left as the platform placeholder fails', () => {
  expect(() => loadPublicURL(fixture({ publicURL: '${APP_URL}' }))).toThrow(/TCGREPLAY_PUBLIC_URL.*placeholder/);
});

for (const [what, publicURL] of [
  ['a path', 'https://tcgreplay.app/playmat'],
  ['a trailing slash', 'https://tcgreplay.app/'],
  ['a query', 'https://tcgreplay.app?x=1'],
  ['a fragment', 'https://tcgreplay.app#top'],
]) {
  test(`a public address with ${what} fails`, () => {
    expect(() => loadPublicURL(fixture({ publicURL }))).toThrow(/TCGREPLAY_PUBLIC_URL.*scheme and host only/);
  });
}

test('a plain http public address fails', () => {
  expect(() => loadPublicURL(fixture({ publicURL: 'http://tcgreplay.app' }))).toThrow(/TCGREPLAY_PUBLIC_URL.*https/);
});

test('a public address that is not a URL fails', () => {
  expect(() => loadPublicURL(fixture({ publicURL: 'tcgreplay.app' }))).toThrow(/TCGREPLAY_PUBLIC_URL.*not a URL/);
});

test('a PRIMARY domain that differs from the public address fails, naming both', () => {
  const run = () => loadPublicURL(fixture({ primary: 'tcgreplay.demo.gs' }));
  expect(run).toThrow(/https:\/\/tcgreplay\.app/);
  expect(run).toThrow(/PRIMARY domain is "tcgreplay\.demo\.gs"/);
});

test('a definition with no PRIMARY domain fails', () => {
  expect(() => loadPublicURL(fixture({ primary: null }))).toThrow(/no domain with type PRIMARY/);
});

test('parseGoDuration reads Go durations in milliseconds', () => {
  expect(parseGoDuration('45s')).toBe(45_000);
  expect(parseGoDuration('1m')).toBe(60_000);
  expect(parseGoDuration('60m')).toBe(3_600_000);
  expect(parseGoDuration('90m')).toBe(5_400_000);
  expect(parseGoDuration('2h')).toBe(7_200_000);
  expect(parseGoDuration('1h30m')).toBe(5_400_000);
  expect(parseGoDuration('500ms')).toBe(500);
  expect(parseGoDuration('0')).toBe(0);
  expect(parseGoDuration('')).toBeUndefined();
  expect(parseGoDuration('60')).toBeUndefined();
  expect(parseGoDuration('an hour')).toBeUndefined();
});

test('humanDuration matches the playmat (internal/usergames humanDuration)', () => {
  const cases = [
    ['45s', '45 seconds'],
    ['1s', '1 second'],
    ['1m', '1 minute'],
    ['2m', '2 minutes'],
    ['60m', '60 minutes'],
    ['90m', '90 minutes'],
    ['119m', '119 minutes'],
    ['2h', '2 hours'],
    ['3h', '3 hours'],
  ];
  for (const [go, want] of cases) expect(humanDuration(parseGoDuration(go)), go).toBe(want);
});

test('the shared layout imports the facts, so every page build checks the public address and the limits', () => {
  const base = readFileSync(new URL('../src/layouts/Base.astro', import.meta.url), 'utf8');
  expect(base).toMatch(/^import .* from '\.\.\/lib\/facts';$/m);
});

// The workflow check (lib/workflow-pairing.ts): guides and workflow pages
// pair one to one in the playmat's order, and any gap stops the build.
import { pair, parseGuideOrder } from '../src/lib/workflow-pairing.ts';

const goSource = 'var order = []string{"play-game", "review-game"}\n';
const guides = [
  { name: 'review-game', description: 'Use when reviewing.' },
  { name: 'play-game', description: 'Use when playing.' },
];
const page = (guide, prompts = ['A prompt']) => ({ guide, title: `Page for ${guide}`, prompts });

test.describe('workflow pairing', () => {
  test('reads the guide order from guides.go', () => {
    expect(parseGuideOrder(goSource)).toEqual(['play-game', 'review-game']);
    expect(() => parseGuideOrder('package guides\n')).toThrow(/var order/);
  });

  test('pairs every guide with its page, in the guide order', () => {
    const got = pair(guides, [page('review-game'), page('play-game')], ['play-game', 'review-game']);
    expect(got.map((w) => [w.name, w.description, w.page.title])).toEqual([
      ['play-game', 'Use when playing.', 'Page for play-game'],
      ['review-game', 'Use when reviewing.', 'Page for review-game'],
    ]);
  });

  test('a guide with no page stops the build, naming it', () => {
    expect(() => pair(guides, [page('play-game')], ['play-game', 'review-game'])).toThrow(/guide "review-game" has no workflow page/);
  });

  test('a page naming a guide the playmat lacks stops the build, naming it', () => {
    expect(() => pair(guides, [page('play-game'), page('review-game'), page('gone-guide')], ['play-game', 'review-game'])).toThrow(
      /names guide "gone-guide", which the playmat does not have/,
    );
  });

  test('a page with no example prompt stops the build', () => {
    expect(() => pair(guides, [page('play-game', []), page('review-game')], ['play-game', 'review-game'])).toThrow(/has no example prompt/);
  });

  test('two pages for one guide stop the build', () => {
    expect(() => pair(guides, [page('play-game'), page('play-game'), page('review-game')], ['play-game', 'review-game'])).toThrow(/more than one workflow page/);
  });
});
