// The coaching workflows, read from the playmat itself so the site can't
// drift from it: the guides' names and descriptions (internal/guides/*.md
// frontmatter) and their order (internal/guides/guides.go). Each guide must
// have exactly one workflow page and each page must name a real guide, or
// the build stops.
import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import { getCollection, type CollectionEntry } from 'astro:content';
import { pair, parseGuideOrder, type Paired } from './workflow-pairing';

export { pair, parseGuideOrder, WorkflowError } from './workflow-pairing';
export type { GuideInfo, WorkflowPage, Paired } from './workflow-pairing';

/** The file that holds the guides' order. Relative to the site project (process.cwd()). */
export const guideOrderPath = join(process.cwd(), '..', 'internal', 'guides', 'guides.go');

/** pairWorkflows reads the guides, the workflow pages and the order, and pairs them (see pair). */
export async function pairWorkflows(): Promise<Paired<CollectionEntry<'workflows'>['data'] & { entry: CollectionEntry<'workflows'> }>[]> {
  const guides = (await getCollection('guides')).map((g) => ({ name: g.data.name, description: g.data.description }));
  const pages = (await getCollection('workflows')).map((entry) => ({ ...entry.data, entry }));
  const order = parseGuideOrder(readFileSync(guideOrderPath, 'utf8'));
  return pair(guides, pages, order);
}
