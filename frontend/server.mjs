import fs from 'node:fs';
import crypto from 'node:crypto';
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
  const nonce = crypto.randomBytes(16).toString('base64');
  const url = new URL(req.url, 'http://localhost');
  if (req.method === 'GET' && url.pathname === '/config.js') {
    res.writeHead(200, {
      ...securityHeaders(nonce),
      'Content-Type': types['.js'],
      'Cache-Control': 'no-store',
    });
    res.end(`window.CITYEVENTS_CONFIG = ${JSON.stringify({ apiBase, ...serviceBases })};\n`);
    return;
  }
  if (!fs.existsSync(root)) {
    res.writeHead(500, { ...securityHeaders(nonce), 'Content-Type': 'text/plain; charset=utf-8' });
    res.end('frontend/dist is missing. Run npm --prefix frontend run build first.');
    return;
  }
  const requested = url.pathname === '/' ? 'index.html' : url.pathname.replace(/^\/+/, '');
  const normalized = path.normalize(requested).replace(/^(\.\.[/\\])+/, '');
  const file = path.join(root, normalized);
  if (!file.startsWith(root)) {
    res.writeHead(403, securityHeaders(nonce));
    res.end();
    return;
  }
  fs.readFile(file, (err, data) => {
    if (err) {
      if (req.method === 'GET' && !path.extname(file)) {
        fs.readFile(path.join(root, 'index.html'), (fallbackErr, fallbackData) => {
          if (fallbackErr) {
            res.writeHead(404, securityHeaders(nonce));
            res.end('not found');
            return;
          }
          res.writeHead(200, { ...securityHeaders(nonce), 'Content-Type': types['.html'] });
          res.end(htmlWithNonce(fallbackData, nonce));
        });
        return;
      }
      res.writeHead(404, securityHeaders(nonce));
      res.end('not found');
      return;
    }
    const contentType = types[path.extname(file)] || 'application/octet-stream';
    res.writeHead(200, { ...securityHeaders(nonce), 'Content-Type': contentType });
    res.end(path.extname(file) === '.html' ? htmlWithNonce(data, nonce) : data);
  });
}).listen(port, '127.0.0.1', () => {
  console.log(`Frontend listening on http://127.0.0.1:${port}`);
  console.log(`Frontend API base ${apiBase || '<same-origin>'}`);
});

function cleanBase(value) {
  return String(value || '').trim().replace(/\/+$/, '');
}

function htmlWithNonce(data, nonce) {
  return data.toString('utf8').replace(/<script\b/g, `<script nonce="${nonce}"`);
}

function securityHeaders(nonce) {
  return {
    'Content-Security-Policy': contentSecurityPolicy(nonce),
    'X-Content-Type-Options': 'nosniff',
    'X-Frame-Options': 'DENY',
    'Referrer-Policy': 'strict-origin-when-cross-origin',
    'Permissions-Policy': 'camera=(), microphone=(), geolocation=()',
  };
}

function contentSecurityPolicy(nonce) {
  const connectSources = Array.from(new Set([
    "'self'",
    apiBase,
    ...Object.values(serviceBases),
    'http://127.0.0.1:8080',
    'http://localhost:8080',
    'http://cityevents.local',
    'https://cityevents.local',
  ].filter(Boolean)));
  return [
    "default-src 'self'",
    `script-src 'self' 'nonce-${nonce}'`,
    "style-src 'self' https://fonts.googleapis.com",
    "font-src 'self' https://fonts.gstatic.com",
    "img-src 'self' data: blob:",
    `connect-src ${connectSources.join(' ')}`,
    "object-src 'none'",
    "base-uri 'self'",
    "frame-ancestors 'none'",
    "form-action 'self'",
  ].join('; ');
}
