import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import { VitePWA } from 'vite-plugin-pwa'
import { fileURLToPath } from 'node:url'

const frontendPort = parseInt(process.env.PORT) || 4533
const backendPort = frontendPort + 100
const sourceMapsEnabled = process.env.ND_UI_SOURCEMAP !== 'false'

// https://vitejs.dev/config/
export default defineConfig({
  resolve: {
    alias: [
      {
        // The package's CommonJS build asks SortableJS for a named Swap
        // export that its CommonJS entry does not expose. Use the package's
        // ESM build together with Sortable's plugin-aware ESM entry.
        find: /^navidrome-music-player$/,
        replacement: fileURLToPath(
          new URL(
            './node_modules/navidrome-music-player/es/index.js',
            import.meta.url,
          ),
        ),
      },
      {
        // navidrome-music-player imports Sortable's named Swap plugin. The
        // package's CommonJS entry only exposes the default Sortable class,
        // which makes the player throw during application startup.
        find: /^sortablejs$/,
        replacement: fileURLToPath(
          new URL(
            './node_modules/sortablejs/modular/sortable.esm.js',
            import.meta.url,
          ),
        ),
      },
      {
        // React-admin 5.15 still emits a handful of MUI 5 compatibility props
        // together with slotProps. Normalize its exact barrel import for MUI 9.
        find: /^@mui\/material$/,
        replacement: fileURLToPath(
          new URL('./src/compat/muiMaterial.jsx', import.meta.url),
        ),
      },
      {
        find: /^@mui\/icons-material$/,
        replacement: fileURLToPath(
          new URL('./src/compat/muiIcons.js', import.meta.url),
        ),
      },
    ],
  },
  plugins: [
    react(),
    VitePWA({
      manifest: manifest(),
      strategies: 'injectManifest',
      srcDir: 'src',
      filename: 'sw.js',
      injectManifest: {
        maximumFileSizeToCacheInBytes: 3 * 1024 * 1024, // 3 MiB
        // index.html is rendered per-user by the server, so a precached copy
        // would pin one user's config (and auth payload) across logins
        globIgnores: ['index.html'],
      },
      devOptions: {
        enabled: true,
      },
    }),
  ],
  server: {
    host: true,
    port: frontendPort,
    proxy: {
      '^/(auth|api|rest|backgrounds)/.*': 'http://localhost:' + backendPort,
    },
  },
  base: './',
  define: {
    // JSONForms and other libraries use process.env
    'process.env': JSON.stringify({}),
  },
  build: {
    outDir: 'build',
    sourcemap: sourceMapsEnabled,
  },
  test: {
    // Vitest resolves its own alias table instead of inheriting Vite's table.
    alias: [
      {
        find: /^@mui\/material$/,
        replacement: fileURLToPath(
          new URL('./src/compat/muiMaterial.jsx', import.meta.url),
        ),
      },
    ],
    globals: true,
    environment: 'jsdom',
    setupFiles: './src/setupTests.js',
    css: true,
    reporters: ['verbose'],
    server: {
      deps: {
        inline: [
          'navidrome-music-player',
          'sortablejs',
          'react-admin',
          'ra-ui-materialui',
        ],
      },
    },
    // reporters: ['default', 'hanging-process'],
    coverage: {
      reporter: ['text', 'json', 'html'],
      include: ['src/**/*'],
      exclude: [],
    },
  },
})

// PWA manifest
function manifest() {
  return {
    name: 'Navidrome',
    short_name: 'Navidrome',
    description:
      'Navidrome, an open source web-based music collection server and streamer',
    categories: ['music', 'entertainment'],
    display: 'standalone',
    start_url: './',
    background_color: 'white',
    theme_color: 'blue',
    icons: [
      {
        src: './android-chrome-192x192.png',
        sizes: '192x192',
        type: 'image/png',
      },
      {
        src: './android-chrome-512x512.png',
        sizes: '512x512',
        type: 'image/png',
      },
    ],
  }
}
