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

builds to `needed,<a ...>`, so the page reads "needed,refresh". The same happens before `<strong>`, `<code>`, `<span>` and `<time>`, and before a `{expression}` that starts the next line: "the project's" followed by `{notice.help.label}` on the next line rendered as "project'sGitHub". Markdown docs are not affected. The build passes and most tests miss it, because Playwright's `toContainText` and text extraction often normalise whitespace.

To avoid it, keep the inline element on the same line as the text before it, wrapping the source line *after* the element or inside its text rather than before its opening tag. To catch it, grep the built HTML for a letter or punctuation mark directly followed by `<a`, `<strong>` or `<code>`:

`grep -oE '[A-Za-z,.;:]<(a|strong|code)[ >]' internal/site/dist/app/<page>/index.html`

Found on the connecting, What's new, Example and Terms pages (all fixed in the Studio restyle, November 2026). Grep a page's built HTML after editing it, and use `{' '}` where a line must break before an inline element or expression.

**It also works the other way round.** A line ending with a closing inline tag, followed by text on the next line, loses its space too: `<code>openid</code>` then "and" on the next line rendered "openidand". So grep the built page both ways:

`grep -oE "[A-Za-z,.;:'’]<(a|strong|code|span|time)[ >]" <page>` and `grep -oE "</(a|strong|code|span|time)>[A-Za-z(]" <page>`
