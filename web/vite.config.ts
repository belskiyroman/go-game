import { defineConfig } from 'vite';
import solid from 'vite-plugin-solid';

// In development the Go server runs on :3000 and Vite proxies the API and the WebSocket to it.
export default defineConfig({
  plugins: [solid()],
  server: {
    port: 5173,
    proxy: {
      '/api': 'http://localhost:3000',
      '/ws': { target: 'ws://localhost:3000', ws: true },
    },
  },
  build: { target: 'es2022' },
});
