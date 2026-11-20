// The example game: the anonymised battle log committed beside the site
// (content/example/battle-log.txt, guarded by internal/game's
// TestSiteExampleGame) and its decklist, read from the test corpus at build
// time rather than copied. Paths are relative to the site project
// (process.cwd()), as in lib/facts.ts.
import { readFileSync } from 'node:fs';
import { join } from 'node:path';

const deckPath = join(process.cwd(), '..', 'testdata', 'decks', 'mega-lucario-ex-v12.txt');
const logPath = join(process.cwd(), 'src', 'content', 'example', 'battle-log.txt');

function read(path: string, what: string): string {
  try {
    return readFileSync(path, 'utf8');
  } catch (err) {
    throw new Error(`site example: cannot read the ${what} at ${path}: ${(err as Error).message}`);
  }
}

/** Text as it is copied: without the file's final newline. */
const asCopied = (s: string) => s.replace(/\r?\n$/, '');

/** The example game's battle log and decklist. */
export const example = {
  log: asCopied(read(logPath, 'example battle log')),
  deck: asCopied(read(deckPath, 'example decklist')),
};
