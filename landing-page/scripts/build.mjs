import { readFileSync, writeFileSync, readdirSync, copyFileSync, rmSync, mkdirSync, statSync } from 'fs';
import { join, dirname } from 'path';
import { fileURLToPath } from 'url';
import { minify } from 'html-minifier-terser';

const __dirname = dirname(fileURLToPath(import.meta.url));
const rootDir = join(__dirname, '..');
const srcDir = join(rootDir, 'src');
const distDir = join(rootDir, 'dist');

// rm -rf dist then recreate
try {
  rmSync(distDir, { recursive: true, force: true });
} catch (err) {
  console.error('Failed to remove dist:', err.message);
  process.exit(1);
}

try {
  mkdirSync(distDir, { recursive: true });
} catch (err) {
  console.error('Failed to create dist:', err.message);
  process.exit(1);
}

// Minify index.html
const srcHtml = join(srcDir, 'index.html');
const distHtml = join(distDir, 'index.html');

let htmlSource;
try {
  htmlSource = readFileSync(srcHtml, 'utf8');
} catch (err) {
  console.error('Failed to read src/index.html:', err.message);
  process.exit(1);
}

let htmlMinified;
try {
  htmlMinified = await minify(htmlSource, {
    collapseWhitespace: true,
    removeComments: true,
    minifyCSS: true,
    minifyJS: true,
    removeRedundantAttributes: true,
    removeScriptTypeAttributes: true,
    removeStyleLinkTypeAttributes: true,
    useShortDoctype: true,
    sortAttributes: true,
    sortClassName: true,
  });
} catch (err) {
  console.error('Failed to minify index.html:', err.message);
  process.exit(1);
}

try {
  writeFileSync(distHtml, htmlMinified, 'utf8');
} catch (err) {
  console.error('Failed to write dist/index.html:', err.message);
  process.exit(1);
}

const origSize = Buffer.byteLength(htmlSource, 'utf8');
const minSize = Buffer.byteLength(htmlMinified, 'utf8');
const reduction = (((origSize - minSize) / origSize) * 100).toFixed(1);
console.log(`index.html: ${origSize} bytes → ${minSize} bytes (${reduction}% reduction)`);

// Copy every other file in src/ verbatim into dist/
let srcEntries;
try {
  srcEntries = readdirSync(srcDir);
} catch (err) {
  console.error('Failed to read src/:', err.message);
  process.exit(1);
}

for (const entry of srcEntries) {
  if (entry === 'index.html') continue;

  const srcPath = join(srcDir, entry);
  const distPath = join(distDir, entry);

  try {
    const info = statSync(srcPath);
    if (!info.isFile()) continue; // skip sub-directories
    copyFileSync(srcPath, distPath);
    console.log(`Copied: src/${entry} → dist/${entry}`);
  } catch (err) {
    console.error(`Failed to copy ${entry}:`, err.message);
    process.exit(1);
  }
}

console.log('Build complete.');
