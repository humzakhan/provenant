import { createServer } from 'node:http';
import { watch } from 'node:fs';
import { readFile } from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const root = path.resolve(__dirname, '..');
const srcDir = path.join(root, 'src');
const port = Number(process.env.PORT) || 8788;

const MIME = {
  '.html': 'text/html; charset=utf-8',
  '.css': 'text/css; charset=utf-8',
  '.js': 'text/javascript; charset=utf-8',
  '.mjs': 'text/javascript; charset=utf-8',
  '.json': 'application/json; charset=utf-8',
  '.svg': 'image/svg+xml',
  '.png': 'image/png',
  '.jpg': 'image/jpeg',
  '.jpeg': 'image/jpeg',
  '.gif': 'image/gif',
  '.webp': 'image/webp',
  '.ico': 'image/x-icon',
  '.woff': 'font/woff',
  '.woff2': 'font/woff2',
  '.txt': 'text/plain; charset=utf-8',
};

// Snippet injected into served HTML: opens an SSE channel and reloads on change.
const LIVE_RELOAD = `
<script>
(function () {
  var es = new EventSource('/__livereload');
  es.onmessage = function () { location.reload(); };
  es.onerror = function () { es.close(); setTimeout(function () { location.reload(); }, 1000); };
})();
</script>`;

// Connected SSE clients to notify on file changes.
const clients = new Set();

function notifyReload() {
  for (const res of clients) {
    try { res.write('data: reload\n\n'); } catch { /* client gone */ }
  }
}

// Resolve a request path to a file inside srcDir, preventing path traversal.
function resolvePath(urlPath) {
  const decoded = decodeURIComponent(urlPath.split('?')[0]);
  let rel = decoded === '/' ? '/index.html' : decoded;
  if (rel.endsWith('/')) rel += 'index.html';
  const full = path.normalize(path.join(srcDir, rel));
  if (full !== srcDir && !full.startsWith(srcDir + path.sep)) return null; // traversal guard
  return full;
}

const server = createServer(async (req, res) => {
  // Live-reload event stream.
  if (req.url === '/__livereload') {
    res.writeHead(200, {
      'Content-Type': 'text/event-stream',
      'Cache-Control': 'no-cache',
      Connection: 'keep-alive',
    });
    res.write('retry: 1000\n\n');
    clients.add(res);
    req.on('close', () => clients.delete(res));
    return;
  }

  const filePath = resolvePath(req.url);
  if (!filePath) { res.writeHead(403); res.end('Forbidden'); return; }

  try {
    const ext = path.extname(filePath).toLowerCase();
    const type = MIME[ext] || 'application/octet-stream';
    if (ext === '.html') {
      let html = await readFile(filePath, 'utf8');
      html = html.includes('</body>')
        ? html.replace('</body>', LIVE_RELOAD + '\n</body>')
        : html + LIVE_RELOAD;
      res.writeHead(200, { 'Content-Type': type, 'Cache-Control': 'no-store' });
      res.end(html);
    } else {
      const buf = await readFile(filePath);
      res.writeHead(200, { 'Content-Type': type, 'Cache-Control': 'no-store' });
      res.end(buf);
    }
  } catch {
    res.writeHead(404, { 'Content-Type': 'text/html; charset=utf-8' });
    res.end('<h1>404</h1><p>Not found: ' + req.url + '</p>');
  }
});

// Watch src/ and tell connected browsers to reload (debounced).
let pending = null;
watch(srcDir, { recursive: true }, (_event, file) => {
  if (file) console.log(`Changed: ${file}`);
  clearTimeout(pending);
  pending = setTimeout(notifyReload, 80);
});

server.listen(port, () => {
  console.log(`\nServing ${path.relative(root, srcDir)}/ with live reload at http://localhost:${port}\n(edit any file in src/ and the browser reloads automatically; Ctrl+C to stop)\n`);
});

function shutdown() { server.close(); process.exit(0); }
process.on('SIGINT', shutdown);
process.on('SIGTERM', shutdown);
