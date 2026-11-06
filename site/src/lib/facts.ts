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

/** The App Platform definition, where the hosted limits are set. */
export const appSpecPath = join(process.cwd(), '..', '.do', 'app.yaml');

const publicURL = 'https://tcgreplay.demo.gs';

/** The playmat's public addresses and the source repository. */
export const site = {
  publicURL,
  mcpURL: `${publicURL}/mcp`,
  boardURL: `${publicURL}/board`,
  repoURL: 'https://github.com/nicholasjackson/tcgreplay',
} as const;

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

/** The hosted limits, read once when the site builds. */
export const limits: Limits = loadLimits(readAppSpec());
