// The public website, built to plain HTML into the Go package that embeds
// it (internal/site; go:embed cannot read parent directories). Astro empties
// outDir on every build; the placeholder lives beside it in dist/placeholder.
import { defineConfig } from 'astro/config';

export default defineConfig({
  site: 'https://tcgreplay.demo.gs',
  outDir: '../internal/site/dist/app',
  trailingSlash: 'never',
  build: { format: 'directory' },
  markdown: { syntaxHighlight: false },
  devToolbar: { enabled: false },
});
