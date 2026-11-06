// Content collections.
//   docs: the how-to pages under src/content/docs (top level only),
//     rendered at /docs/<id> by pages/docs/[...slug].astro.
//   guides: the playmat's coaching guides (internal/guides/*.md), read for
//     their name and one-line description; the site never edits them.
//   workflows: one hand-written page per guide, under
//     src/content/docs/workflows, rendered at /docs/workflows/<guide>.
// lib/workflows.ts pairs guides and workflow pages and fails the build on
// any gap.
import { defineCollection } from 'astro:content';
import { glob } from 'astro/loaders';
import { z } from 'astro/zod';

const docs = defineCollection({
  loader: glob({ pattern: '*.md', base: './src/content/docs' }),
  schema: z.object({
    title: z.string(),
    description: z.string().optional(),
    // Position in the docs sidebar, lowest first.
    order: z.number().default(100),
    // Show the playmat's MCP address, with a copy button, above the page.
    showAddress: z.boolean().default(false),
  }),
});

const guides = defineCollection({
  loader: glob({ pattern: '*.md', base: '../internal/guides' }),
  schema: z.object({
    name: z.string(),
    description: z.string(),
  }),
});

const workflows = defineCollection({
  loader: glob({ pattern: '*.md', base: './src/content/docs/workflows' }),
  schema: z.object({
    // The guide this page documents: must equal a guide's name.
    guide: z.string(),
    title: z.string(),
    // Example prompts, each shown with a copy button: at least one.
    prompts: z.array(z.string().min(1)).min(1, 'a workflow page needs at least one example prompt'),
  }),
});

export const collections = { docs, guides, workflows };
