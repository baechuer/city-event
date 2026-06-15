import fs from 'node:fs';
import http from 'node:http';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const root = path.join(__dirname, 'dist');
const port = Number(process.argv[2] || 18088);
const apiBase = cleanBase(process.env.CITYEVENTS_API_BASE || 'http://127.0.0.1:8080');
const serviceBases = {
  authBase: cleanBase(process.env.CITYEVENTS_AUTH_BASE || apiBase),
  eventBase: cleanBase(process.env.CITYEVENTS_EVENT_BASE || apiBase),
  feedBase: cleanBase(process.env.CITYEVENTS_FEED_BASE || apiBase),
  mediaBase: cleanBase(process.env.CITYEVENTS_MEDIA_BASE || apiBase),
};
const types = {
  '.html': 'text/html; charset=utf-8',
  '.js': 'text/javascript; charset=utf-8',
  '.css': 'text/css; charset=utf-8',
  '.jpg': 'image/jpeg',
  '.jpeg': 'image/jpeg',
  '.png': 'image/png',
  '.webp': 'image/webp',
  '.map': 'application/json; charset=utf-8',
};

http.createServer((req, res) => {
  const url = new URL(req.url, 'http://localhost');
  if (req.method === 'GET' && url.pathname === '/config.js') {
    res.writeHead(200, {
      'Content-Type': types['.js'],
      'Cache-Control': 'no-store',
    });
    res.end(`window.CITYEVENTS_CONFIG = ${JSON.stringify({ apiBase, ...serviceBases })};\n`);
    return;
  }
  if (!fs.existsSync(root)) {
    res.writeHead(500, { 'Content-Type': 'text/plain; charset=utf-8' });
    res.end('frontend/dist is missing. Run npm --prefix frontend run build first.');
    return;
  }
  const requested = url.pathname === '/' ? 'index.html' : url.pathname.replace(/^\/+/, '');
  const normalized = path.normalize(requested).replace(/^(\.\.[/\\])+/, '');
  const file = path.join(root, normalized);
  if (!file.startsWith(root)) {
    res.writeHead(403);
    res.end();
    return;
  }
  fs.readFile(file, (err, data) => {
    if (err) {
      if (req.method === 'GET' && !path.extname(file)) {
        fs.readFile(path.join(root, 'index.html'), (fallbackErr, fallbackData) => {
          if (fallbackErr) {
            res.writeHead(404);
            res.end('not found');
            return;
          }
          res.writeHead(200, { 'Content-Type': types['.html'] });
          res.end(fallbackData);
        });
        return;
      }
      res.writeHead(404);
      res.end('not found');
      return;
    }
    res.writeHead(200, { 'Content-Type': types[path.extname(file)] || 'application/octet-stream' });
    res.end(data);
  });
}).listen(port, '127.0.0.1', () => {
  console.log(`Frontend listening on http://127.0.0.1:${port}`);
  console.log(`Frontend API base ${apiBase || '<same-origin>'}`);
});

function cleanBase(value) {
  return String(value || '').trim().replace(/\/+$/, '');
}
