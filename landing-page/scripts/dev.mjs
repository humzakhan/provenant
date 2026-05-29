import path from 'node:path';
import { fileURLToPath } from 'node:url';
import browserSync from 'browser-sync';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const root = path.resolve(__dirname, '..');
const srcDir = path.join(root, 'src');
const port = Number(process.env.PORT) || 8788;

const bs = browserSync.create();

bs.init({
  server: { baseDir: srcDir },
  port,
  files: [
    // Watch real source files only; ignore atomic-save temp files (e.g.
    // index.html.tmp.NNNN) that editors create and immediately delete.
    path.join(srcDir, '**/*.{html,css,js,mjs,json,svg,png,jpg,jpeg,gif,webp,ico,woff,woff2}'),
  ],
  watchOptions: {
    ignored: ['**/*.tmp.*', '**/*.swp', '**/*~', '**/.DS_Store'],
    ignoreInitial: true,
  },
  ignore: ['**/*.tmp.*'],
  open: false,
  notify: false,
  ui: false,
  logPrefix: 'dev',
  logFileChanges: true,
});
