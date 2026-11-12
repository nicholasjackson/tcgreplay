// Facts the playmat defines, read from its own sources while the site
// builds so the pages can't drift from what is deployed. A missing or
// unreadable fact fails the build with a message naming it.
//
// Paths are relative to the site project (process.cwd() during astro
// build and the browser suite), never import.meta.url, which points into
// Astro's prerender bundle at build time.
import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import { parse } from 'yaml';

/** The App Platform definition, where the public address and the hosted limits are set. */
export const appSpecPath = join(process.cwd(), '..', '.do', 'app.yaml');

/** The hosted playmat's limits, as the App Platform definition sets them. */
export type Limits = {
  /** Requests a minute per account (TCGREPLAY_RATE_LIMIT). */
  requestsPerMinute: number;
  /** Signed-in accounts served at once (TCGREPLAY_MAX_USERS). */
  maxUsers: number;
  /** How long a game may go without activity, for people, e.g. "60 minutes" (TCGREPLAY_IDLE_TIMEOUT). */
  idleTimeout: string;
};

const envKeys = {
  requestsPerMinute: 'TCGREPLAY_RATE_LIMIT',
  maxUsers: 'TCGREPLAY_MAX_USERS',
  idleTimeout: 'TCGREPLAY_IDLE_TIMEOUT',
} as const;

class FactError extends Error {
  constructor(message: string) {
    super(`site facts: ${message}`);
    this.name = 'FactError';
  }
}

/**
 * loadLimits reads the playmat service's limits from an App Platform
 * definition (YAML text). It throws if any is missing or unparsable.
 */
export function loadLimits(yamlText: string, source = '.do/app.yaml'): Limits {
  const { value } = readSpec(yamlText, source);
  const count = (key: string): number => {
    const v = value(key);
    const n = Number(v);
    if (!/^\d+$/.test(v) || !Number.isSafeInteger(n) || n <= 0) {
      throw new FactError(`${source} sets ${key} to "${v}"; the website needs a whole number above 0`);
    }
    return n;
  };
  const idle = value(envKeys.idleTimeout);
  const ms = parseGoDuration(idle);
  if (ms === undefined || ms <= 0) {
    throw new FactError(`${source} sets ${envKeys.idleTimeout} to "${idle}"; the website needs a Go duration above 0, such as 60m`);
  }
  return {
    requestsPerMinute: count(envKeys.requestsPerMinute),
    maxUsers: count(envKeys.maxUsers),
    idleTimeout: humanDuration(ms),
  };
}

/**
 * readSpec parses an App Platform definition and finds the playmat
 * service. Its value function returns an env value of that service, or
 * throws naming the key when it is unset or empty.
 */
function readSpec(yamlText: string, source: string): { spec: any; value: (key: string) => string } {
  let spec: any;
  try {
    spec = parse(yamlText);
  } catch (err) {
    throw new FactError(`${source} is not valid YAML: ${(err as Error).message}`);
  }
  const service = Array.isArray(spec?.services) ? spec.services.find((s: any) => s?.name === 'playmat') : undefined;
  if (!service) {
    throw new FactError(`${source} has no service named "playmat"`);
  }
  const envs: any[] = Array.isArray(service.envs) ? service.envs : [];
  const value = (key: string): string => {
    const env = envs.find((e) => e?.key === key);
    if (env === undefined || env.value === undefined || env.value === null || String(env.value).trim() === '') {
      throw new FactError(`${source} does not set ${key} on the playmat service; the website states it, so the build stops`);
    }
    return String(env.value).trim();
  };
  return { spec, value };
}

const publicURLKey = 'TCGREPLAY_PUBLIC_URL';

/**
 * loadPublicURL reads the playmat's one public address (TCGREPLAY_PUBLIC_URL)
 * from an App Platform definition (YAML text), the same value the server
 * uses. It must be a bare https address, scheme and host only with no
 * trailing slash, whose host is the domain listed with type PRIMARY.
 * Otherwise it throws, naming the problem.
 */
export function loadPublicURL(yamlText: string, source = '.do/app.yaml'): string {
  const { spec, value } = readSpec(yamlText, source);
  const raw = value(publicURLKey);
  if (raw.includes('${')) {
    throw new FactError(`${source} sets ${publicURLKey} to "${raw}", which is a platform placeholder; set the literal address, such as https://tcgreplay.app`);
  }
  let u: URL;
  try {
    u = new URL(raw);
  } catch {
    throw new FactError(`${source} sets ${publicURLKey} to "${raw}", which is not a URL; it must be https:// and a host`);
  }
  if (u.protocol !== 'https:' || !u.hostname) {
    throw new FactError(`${source} sets ${publicURLKey} to "${raw}"; it must be an https:// address with a host`);
  }
  if (u.username || u.password || u.search || u.hash || raw !== `${u.protocol}//${u.host}`) {
    throw new FactError(
      `${source} sets ${publicURLKey} to "${raw}"; it must be the scheme and host only, with no path, query, fragment or trailing slash`,
    );
  }
  const domains: any[] = Array.isArray(spec?.domains) ? spec.domains : [];
  const primary = domains.find((d) => d?.type === 'PRIMARY');
  if (!primary || !primary.domain) {
    throw new FactError(`${source} has no domain with type PRIMARY; ${publicURLKey} (${raw}) must name the primary domain`);
  }
  if (String(primary.domain).toLowerCase() !== u.host) {
    throw new FactError(
      `${source} sets ${publicURLKey} to "${raw}", but the PRIMARY domain is "${primary.domain}"; the two must name the same host`,
    );
  }
  return raw;
}

const unitMs: Record<string, number> = { ns: 1e-6, us: 1e-3, 'µs': 1e-3, ms: 1, s: 1000, m: 60_000, h: 3_600_000 };

/** parseGoDuration parses a Go duration ("60m", "1h30m", "90s") into milliseconds, or undefined. */
export function parseGoDuration(s: string): number | undefined {
  if (s === '0') return 0;
  const re = /(\d+(?:\.\d+)?)(ns|us|µs|ms|s|m|h)/gy;
  let total = 0;
  let matched = 0;
  let m: RegExpExecArray | null;
  while ((m = re.exec(s)) !== null) {
    total += Number(m[1]) * unitMs[m[2]];
    matched = re.lastIndex;
  }
  return matched === s.length && s.length > 0 ? total : undefined;
}

/**
 * humanDuration renders an idle time for people, as the playmat does
 * (internal/usergames humanDuration): seconds under a minute, minutes
 * under two hours, else hours.
 */
export function humanDuration(ms: number): string {
  const plural = (n: number, unit: string) => (n === 1 ? `1 ${unit}` : `${n} ${unit}s`);
  if (ms < 60_000) return plural(Math.floor(ms / 1000), 'second');
  if (ms < 2 * 3_600_000) return plural(Math.floor(ms / 60_000), 'minute');
  return plural(Math.floor(ms / 3_600_000), 'hour');
}

function readAppSpec(): string {
  try {
    return readFileSync(appSpecPath, 'utf8');
  } catch (err) {
    throw new FactError(`cannot read ${appSpecPath}: ${(err as Error).message}`);
  }
}

const appSpec = readAppSpec();

/** The hosted limits, read once when the site builds. */
export const limits: Limits = loadLimits(appSpec);

const publicURL = loadPublicURL(appSpec);

/**
 * The playmat's public addresses, derived from TCGREPLAY_PUBLIC_URL in the
 * App Platform definition, and the source repository.
 */
export const site = {
  publicURL,
  mcpURL: `${publicURL}/mcp`,
  boardURL: `${publicURL}/board`,
  repoURL: 'https://github.com/nicholasjackson/tcgreplay',
} as const;
