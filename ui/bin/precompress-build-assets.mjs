import { brotliCompressSync, constants, gzipSync } from 'node:zlib'
import { readdirSync, readFileSync, statSync, writeFileSync } from 'node:fs'
import { join, extname } from 'node:path'
import { fileURLToPath } from 'node:url'

const buildDir = fileURLToPath(new URL('../build', import.meta.url))
const compressibleExtensions = new Set([
  '.css',
  '.html',
  '.js',
  '.json',
  '.map',
  '.svg',
  '.txt',
  '.wasm',
  '.webmanifest',
  '.xml',
])

const minStaticPrecompressSize = 256
const brotliLargeFileThreshold = 1024 * 1024

function walk(dir) {
  for (const entry of readdirSync(dir, { withFileTypes: true })) {
    const file = join(dir, entry.name)
    if (entry.isDirectory()) {
      walk(file)
      continue
    }
    if (!entry.isFile()) continue
    if (entry.name.endsWith('.br') || entry.name.endsWith('.gz')) continue
    if (!compressibleExtensions.has(extname(entry.name))) continue

    const stat = statSync(file)
    if (stat.size < minStaticPrecompressSize) continue

    const input = readFileSync(file)
    const brotliQuality = stat.size >= brotliLargeFileThreshold ? 9 : 11
    const brotli = brotliCompressSync(input, {
      params: {
        [constants.BROTLI_PARAM_QUALITY]: brotliQuality,
        [constants.BROTLI_PARAM_MODE]: constants.BROTLI_MODE_TEXT,
      },
    })
    const gzip = gzipSync(input, { level: 9 })

    if (brotli.length < input.length) {
      writeFileSync(`${file}.br`, brotli)
    }
    if (gzip.length < input.length) {
      writeFileSync(`${file}.gz`, gzip)
    }
  }
}

walk(buildDir)
