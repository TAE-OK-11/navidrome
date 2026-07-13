import { brotliCompress, constants, gzip, zstdCompress } from 'node:zlib'
import { availableParallelism } from 'node:os'
import { readdir, readFile, stat, writeFile } from 'node:fs/promises'
import { extname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { promisify } from 'node:util'

const compressBrotli = promisify(brotliCompress)
const compressGzip = promisify(gzip)
const compressZstd = promisify(zstdCompress)

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
const maxCompressionJobs = 4
const requestedJobs = Number.parseInt(
  process.env.STATIC_COMPRESSION_JOBS ?? '',
  10,
)
const compressionJobs = Math.max(
  1,
  Math.min(
    Number.isFinite(requestedJobs) && requestedJobs > 0
      ? requestedJobs
      : availableParallelism(),
    maxCompressionJobs,
  ),
)

async function collectFiles(dir, files = []) {
  for (const entry of await readdir(dir, { withFileTypes: true })) {
    const file = join(dir, entry.name)
    if (entry.isDirectory()) {
      await collectFiles(file, files)
      continue
    }
    if (!entry.isFile()) continue
    if (
      entry.name.endsWith('.br') ||
      entry.name.endsWith('.gz') ||
      entry.name.endsWith('.zst')
    ) {
      continue
    }
    if (compressibleExtensions.has(extname(entry.name))) files.push(file)
  }
  return files
}

async function compressFile(file) {
  const fileStat = await stat(file)
  if (fileStat.size < minStaticPrecompressSize) return [0, 0, 0]

  const input = await readFile(file)
  const extension = extname(file)
  const brotliQuality = fileStat.size >= brotliLargeFileThreshold ? 9 : 11
  const brotliMode =
    extension === '.wasm'
      ? constants.BROTLI_MODE_GENERIC
      : constants.BROTLI_MODE_TEXT

  const [brotli, zstd, gzipData] = await Promise.all([
    compressBrotli(input, {
      params: {
        [constants.BROTLI_PARAM_QUALITY]: brotliQuality,
        [constants.BROTLI_PARAM_MODE]: brotliMode,
        [constants.BROTLI_PARAM_SIZE_HINT]: input.length,
      },
    }),
    compressZstd(input, {
      params: {
        [constants.ZSTD_c_compressionLevel]: 19,
        [constants.ZSTD_c_checksumFlag]: 1,
      },
    }),
    compressGzip(input, { level: 9 }),
  ])

  const outputs = [
    [brotli, `${file}.br`],
    [zstd, `${file}.zst`],
    [gzipData, `${file}.gz`],
  ].filter(([data]) => data.length < input.length)

  await Promise.all(outputs.map(([data, output]) => writeFile(output, data)))
  return [
    Number(brotli.length < input.length),
    Number(zstd.length < input.length),
    Number(gzipData.length < input.length),
  ]
}

const files = await collectFiles(buildDir)
const totals = [0, 0, 0]
let nextFile = 0

await Promise.all(
  Array.from({ length: Math.min(compressionJobs, files.length) }, async () => {
    while (nextFile < files.length) {
      const file = files[nextFile++]
      const counts = await compressFile(file)
      for (const index of counts.keys()) totals[index] += counts[index]
    }
  }),
)

console.log(
  `[precompress] jobs=${compressionJobs} brotli=${totals[0]} zstd=${totals[1]} gzip=${totals[2]}`,
)
