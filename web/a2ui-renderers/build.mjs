// Builds the optional A2UI web bundles that ChatGo embeds.
//
// The host loads each bundle from a2ui-web://<token>/bundle.js. The bundle must
// be one self-contained file with no external imports, because the renderer
// page runs under a strict Content-Security-Policy.
import { build } from 'esbuild'
import { mkdir, rm } from 'node:fs/promises'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

const here = dirname(fileURLToPath(import.meta.url))
const outDir = resolve(here, '../../pkg/a2ui/web')

const bundles = [
  { entry: 'src/react.tsx', name: 'bundle.react' },
  { entry: 'src/copilot.tsx', name: 'bundle.copilot' },
]

await mkdir(outDir, { recursive: true })
for (const { entry, name } of bundles) {
  for (const ext of ['.js', '.css']) {
    await rm(resolve(outDir, name + ext), { force: true })
  }
  await build({
    absWorkingDir: here,
    entryPoints: [resolve(here, entry)],
    outfile: resolve(outDir, name + '.js'),
    bundle: true,
    format: 'iife',
    platform: 'browser',
    target: ['safari16', 'chrome108', 'firefox110'],
    jsx: 'automatic',
    minify: true,
    sourcemap: false,
    legalComments: 'eof',
    define: {
      'process.env.NODE_ENV': '"production"',
    },
    loader: {
      '.css': 'css',
      '.woff': 'file',
      '.woff2': 'file',
      '.ttf': 'file',
    },
  })
  console.log(`${name}.js written`)
}