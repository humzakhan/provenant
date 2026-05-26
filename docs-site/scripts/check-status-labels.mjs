import { readdir, readFile } from 'node:fs/promises';
import { join } from 'node:path';

const root = new URL('../src/content/docs/', import.meta.url);
const labels = [
  'Implemented now',
  'Architecture contract',
  'Planned',
  'Deferred',
];

async function* files(dir) {
  for (const entry of await readdir(dir, { withFileTypes: true })) {
    const path = join(dir, entry.name);
    if (entry.isDirectory()) {
      yield* files(path);
    } else if (entry.name.endsWith('.md') || entry.name.endsWith('.mdx')) {
      yield path;
    }
  }
}

const missing = [];
for await (const file of files(root.pathname)) {
  const text = await readFile(file, 'utf8');
  if (!labels.some((label) => text.includes(label))) {
    missing.push(file.replace(root.pathname, ''));
  }
}

if (missing.length > 0) {
  console.error('Missing content status label:');
  for (const file of missing) console.error(`- ${file}`);
  process.exit(1);
}
