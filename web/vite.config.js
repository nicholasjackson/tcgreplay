import { defineConfig } from 'vite';
import { svelte } from '@sveltejs/vite-plugin-svelte';

const addr = process.env.TCGREPLAY_ADDR || '127.0.0.1:7777';
// TCGREPLAY_LAN=1 serves the dev board to other devices on the network (a
// phone, a tablet). The daemon answers only loopback Host and Origin
// headers, so the proxy rewrites both; Vite's own host check still turns
// away DNS rebinding. Anyone on the network can then use the board.
const lan = !!process.env.TCGREPLAY_LAN;
const asLocal = lan ? { changeOrigin: true, headers: { origin: `http://${addr}` } } : {};

// The build lands inside the Go package so go:embed can reach it
// (go:embed cannot read parent directories).
export default defineConfig({
  plugins: [svelte()],
  build: {
    outDir: '../internal/webui/dist/app',
    emptyOutDir: true,
  },
  server: {
    host: lan || undefined,
    // `npm run dev` proxies to a running tcgreplay binary, at
    // TCGREPLAY_ADDR if set (make dev-ui points it at the dev daemon).
    proxy: {
      '/ws': { target: `ws://${addr}`, ws: true, ...asLocal },
      '/api': { target: `http://${addr}`, ...asLocal },
    },
  },
});
