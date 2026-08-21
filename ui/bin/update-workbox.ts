import { createHash } from 'node:crypto'
import { copyFile, mkdir, readFile } from 'node:fs/promises'
import { dirname, join } from 'node:path'

const root = process.cwd()
const packageJsonPath = join(root, 'package.json')
const sourcePath = join(root, 'node_modules/workbox-sw/build/workbox-sw.js')
const sourceMapPath = `${sourcePath}.map`
const targetPath = join(root, 'public/workbox/workbox-sw.js')
const targetMapPath = `${targetPath}.map`

type PackageJson = {
  dependencies?: Record<string, string>
}

async function md5(path: string): Promise<string | null> {
  try {
    const data = await readFile(path)
    return createHash('md5').update(data).digest('hex')
  } catch (error) {
    if ((error as NodeJS.ErrnoException).code === 'ENOENT') return null
    throw error
  }
}

const packageJson = JSON.parse(
  await readFile(packageJsonPath, 'utf8'),
) as PackageJson
const workboxVersion = packageJson.dependencies?.['workbox-cli'] ?? 'unknown'

const [sourceHash, targetHash] = await Promise.all([
  md5(sourcePath),
  md5(targetPath),
])

if (!sourceHash) {
  throw new Error(`Workbox source not found: ${sourcePath}`)
}

if (sourceHash === targetHash) {
  console.log(`workbox-sw.js is already up-to-date (${workboxVersion})`)
  process.exit(0)
}

await mkdir(dirname(targetPath), { recursive: true })
await Promise.all([
  copyFile(sourcePath, targetPath),
  copyFile(sourceMapPath, targetMapPath),
])
console.log(`Updated workbox-sw.js to ${workboxVersion}`)
