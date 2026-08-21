import { execFileSync } from 'node:child_process'
import { copyFile, mkdir, readdir, rm } from 'node:fs/promises'
import { join } from 'node:path'

const root = process.cwd()
const stagingDir = join(root, 'build/3rdparty')
const targetDir = join(root, 'public/3rdparty/workbox')
const workboxFiles = [
  'workbox-sw.js',
  'workbox-core.prod.js',
  'workbox-strategies.prod.js',
  'workbox-routing.prod.js',
  'workbox-navigation-preload.prod.js',
  'workbox-precaching.prod.js',
] as const

await rm(targetDir, { recursive: true, force: true })

execFileSync('bunx', ['workbox', 'copyLibraries', 'build/3rdparty/'], {
  cwd: root,
  stdio: 'inherit',
})

const entries = await readdir(stagingDir, { withFileTypes: true })
const generatedDirs = entries.filter(
  (entry) => entry.isDirectory() && entry.name.startsWith('workbox-'),
)

if (generatedDirs.length !== 1) {
  throw new Error(
    `Expected exactly one generated Workbox directory, found ${generatedDirs.length}`,
  )
}

const sourceDir = join(stagingDir, generatedDirs[0].name)
await mkdir(targetDir, { recursive: true })
await Promise.all(
  workboxFiles.map((file) =>
    copyFile(join(sourceDir, file), join(targetDir, file)),
  ),
)
await rm(sourceDir, { recursive: true, force: true })
