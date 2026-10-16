import { defineConfig } from 'vite';
import { svelte } from '@sveltejs/vite-plugin-svelte';

// The build lands inside the Go package so go:embed can reach it
// (go:embed cannot read parent directories).
export default defineConfig({
  plugins: [svelte()],
  build: {
    outDir: '../internal/webui/dist/app',
    emptyOutDir: true,
  },
  server: {
    // `npm run dev` proxies to a running tcgreplay binary.
    proxy: {
      '/ws': { target: 'ws://127.0.0.1:7777', ws: true },
      '/api': 'http://127.0.0.1:7777',
    },
  },
});
