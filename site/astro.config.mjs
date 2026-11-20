// The public website, built to plain HTML into the Go package that embeds
// it (internal/site; go:embed cannot read parent directories). Astro empties
// outDir on every build; the placeholder lives beside it in dist/placeholder.
import { defineConfig } from 'astro/config';

export default defineConfig({
  // Must equal TCGREPLAY_PUBLIC_URL in .do/app.yaml. No page reads
  // Astro.site; the pages print the address from src/lib/facts.ts, which
  // reads it from that file and fails the build if it is wrong.
  site: 'https://tcgreplay.app',
  outDir: '../internal/site/dist/app',
  trailingSlash: 'never',
  build: { format: 'directory' },
  markdown: { syntaxHighlight: false },
  devToolbar: { enabled: false },
});
