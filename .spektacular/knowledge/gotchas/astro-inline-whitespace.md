---
tags: [site, astro, layout]
---

# Astro drops the space before an inline element that starts a new source line

In a hand-written `.astro` page, a line break between text and an inline element is dropped, not collapsed to one space. For example:

```astro
<p>
  When an update says a refresh is needed,
  <a href="/docs/connect#refresh">refresh your connection</a>.
</p>
```

builds to `needed,<a ...>`, so the page reads "needed,refresh". The same happens before `<strong>`, `<code>` and `<time>`. Markdown docs are not affected. The build passes and most tests miss it, because Playwright's `toContainText` and text extraction often normalise whitespace.

To avoid it, keep the inline element on the same line as the text before it, wrapping the source line *after* the element or inside its text rather than before its opening tag. To catch it, grep the built HTML for a letter or punctuation mark directly followed by `<a`, `<strong>` or `<code>`:

`grep -oE '[A-Za-z,.;:]<(a|strong|code)[ >]' internal/site/dist/app/<page>/index.html`

Found on the connecting and What's new pages (fixed). `site/src/pages/docs/example.astro` still has one instance ("…see<a href=\"/docs/import\">") as of November 2026.
