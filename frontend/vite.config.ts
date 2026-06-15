import react from '@vitejs/plugin-react';
import { defineConfig, type Plugin } from 'vite';

function cleanBase(value: string | undefined): string {
  return String(value || '').trim().replace(/\/+$/, '');
}

function runtimeConfigPlugin(): Plugin {
  return {
    name: 'cityevents-runtime-config',
    configureServer(server) {
      server.middlewares.use('/config.js', (_req, res) => {
        const apiBase = cleanBase(process.env.CITYEVENTS_API_BASE || 'http://127.0.0.1:8080');
        const config = {
          apiBase,
          authBase: cleanBase(process.env.CITYEVENTS_AUTH_BASE || apiBase),
          eventBase: cleanBase(process.env.CITYEVENTS_EVENT_BASE || apiBase),
          feedBase: cleanBase(process.env.CITYEVENTS_FEED_BASE || apiBase),
          mediaBase: cleanBase(process.env.CITYEVENTS_MEDIA_BASE || apiBase),
        };
        res.setHeader('Content-Type', 'text/javascript; charset=utf-8');
        res.setHeader('Cache-Control', 'no-store');
        res.end(`window.CITYEVENTS_CONFIG = ${JSON.stringify(config)};\n`);
      });
    },
  };
}

export default defineConfig({
  plugins: [react(), runtimeConfigPlugin()],
  server: {
    host: '127.0.0.1',
    port: 18088,
    strictPort: true,
  },
});
