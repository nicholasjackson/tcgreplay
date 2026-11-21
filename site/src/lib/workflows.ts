// The coaching workflows, read from the playmat itself so the site can't
// drift from it: the guides' names and descriptions (internal/guides/*.md
// frontmatter) and their order (internal/guides/guides.go). Each guide must
// have exactly one workflow page and each page must name a real guide, or
// the build stops. The workflows page lists them in groups (each page's
// group frontmatter), in the guides' order within each group.
import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import { getCollection, type CollectionEntry } from 'astro:content';
import { pair, parseGuideOrder, WorkflowError, type Paired } from './workflow-pairing';

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

type PairedWorkflow = Awaited<ReturnType<typeof pairWorkflows>>[number];
export type WorkflowGroupId = CollectionEntry<'workflows'>['data']['group'];

/** The groups the workflows page lists, in the order it lists them, with their headings. */
export const workflowGroups: { id: WorkflowGroupId; title: string }[] = [
  { id: 'after-a-game', title: 'After a game' },
  { id: 'practice', title: 'Practice' },
  { id: 'play', title: 'Play' },
];

/**
 * groupWorkflows is pairWorkflows' list split into workflowGroups, in that
 * order, each keeping the guides' order (internal/guides/guides.go). A
 * group with no workflow is left out.
 */
export async function groupWorkflows(): Promise<{ id: WorkflowGroupId; title: string; workflows: PairedWorkflow[] }[]> {
  const all = await pairWorkflows();
  const known = new Set(workflowGroups.map((g) => g.id));
  const lost = all.filter((w) => !known.has(w.page.group));
  if (lost.length > 0) {
    throw new WorkflowError(lost.map((w) => `the workflow page for "${w.name}" has group "${w.page.group}", which the workflows page does not list`).join('; '));
  }
  return workflowGroups
    .map((g) => ({ ...g, workflows: all.filter((w) => w.page.group === g.id) }))
    .filter((g) => g.workflows.length > 0);
}
