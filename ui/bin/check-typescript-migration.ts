import { readdir, readFile } from 'node:fs/promises'
import { resolve } from 'node:path'

const sourceRoot = resolve(import.meta.dir, '../src')
const maximumNoCheckFiles = 194
const entries = await readdir(sourceRoot, { recursive: true })
const legacyJavaScript = entries.filter(
  (file) => file.endsWith('.js') || file.endsWith('.jsx'),
)

if (legacyJavaScript.length > 0) {
  for (const file of legacyJavaScript) {
    process.stderr.write(
      `${file}: JavaScript source is forbidden; use TypeScript\n`,
    )
  }
  process.exit(1)
}

const sources = entries.filter(
  (file) => file.endsWith('.ts') || file.endsWith('.tsx'),
)
const noCheckFiles: string[] = []

for (const file of sources) {
  const source = await readFile(resolve(sourceRoot, file), 'utf8')
  if (source.includes('@ts-nocheck')) noCheckFiles.push(file)
}

if (noCheckFiles.length > maximumNoCheckFiles) {
  process.stderr.write(
    `TypeScript migration regressed: ${noCheckFiles.length} @ts-nocheck files; maximum is ${maximumNoCheckFiles}\n`,
  )
  process.exit(1)
}

process.stdout.write(
  `TypeScript migration: ${sources.length} typed sources, ${noCheckFiles.length} temporary @ts-nocheck files\n`,
)
