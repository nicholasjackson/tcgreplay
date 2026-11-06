// The pure half of the workflow check: reading the guides' order from
// guides.go and pairing guides with workflow pages. Kept free of Astro
// imports so the browser suite can test it directly (see lib/workflows.ts).

export class WorkflowError extends Error {
  constructor(message: string) {
    super(`site workflows: ${message}`);
    this.name = 'WorkflowError';
  }
}

/** parseGuideOrder reads `var order = []string{...}` from guides.go source. */
export function parseGuideOrder(goSource: string): string[] {
  const m = /var\s+order\s*=\s*\[\]string\s*\{([^}]*)\}/.exec(goSource);
  if (!m) {
    throw new WorkflowError('cannot find `var order = []string{...}` in internal/guides/guides.go');
  }
  const names = [...m[1].matchAll(/"([^"]+)"/g)].map((x) => x[1]);
  if (names.length === 0) {
    throw new WorkflowError('the guide order in internal/guides/guides.go is empty');
  }
  return names;
}

export type GuideInfo = { name: string; description: string };
export type WorkflowPage = { guide: string; title: string; prompts: string[] };
export type Paired<P> = { name: string; description: string; order: number; page: P };

/**
 * pair matches guides to workflow pages one to one, in the guides' order.
 * It throws naming every gap: a guide with no page, a page naming no guide,
 * two pages for one guide, a page with no prompt, or a guide missing from
 * the order.
 */
export function pair<P extends WorkflowPage>(guides: GuideInfo[], pages: P[], order: string[]): Paired<P>[] {
  const problems: string[] = [];
  const byGuide = new Map<string, P>();
  const guideNames = new Set(guides.map((g) => g.name));
  for (const p of pages) {
    if (!guideNames.has(p.guide)) problems.push(`the workflow page "${p.title}" names guide "${p.guide}", which the playmat does not have`);
    else if (byGuide.has(p.guide)) problems.push(`guide "${p.guide}" has more than one workflow page`);
    else byGuide.set(p.guide, p);
    if (!p.prompts || p.prompts.length === 0) problems.push(`the workflow page for "${p.guide}" has no example prompt`);
  }
  for (const g of guides) {
    if (!byGuide.has(g.name)) problems.push(`guide "${g.name}" has no workflow page: add site/src/content/docs/workflows/${g.name}.md`);
    if (!order.includes(g.name)) problems.push(`guide "${g.name}" is not in the order in internal/guides/guides.go`);
  }
  for (const name of order) {
    if (!guideNames.has(name)) problems.push(`the guide order names "${name}", which has no guide file`);
  }
  if (problems.length > 0) {
    throw new WorkflowError(problems.join('; '));
  }
  return order
    .filter((name) => guideNames.has(name))
    .map((name, i) => {
      const g = guides.find((x) => x.name === name)!;
      return { name, description: g.description, order: i, page: byGuide.get(name)! };
    });
}

