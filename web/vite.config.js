import { defineConfig } from 'vite';
import { svelte } from '@sveltejs/vite-plugin-svelte';

const addr = process.env.TCGREPLAY_ADDR || '127.0.0.1:7777';

// The build lands inside the Go package so go:embed can reach it
// (go:embed cannot read parent directories).
export default defineConfig({
  plugins: [svelte()],
  build: {
    outDir: '../internal/webui/dist/app',
    emptyOutDir: true,
  },
  server: {
    // `npm run dev` proxies to a running tcgreplay binary, at
    // TCGREPLAY_ADDR if set (make dev-ui points it at the dev daemon).
    proxy: {
      '/ws': { target: `ws://${addr}`, ws: true },
      '/api': `http://${addr}`,
    },
  },
});
