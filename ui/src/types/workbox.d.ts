/// <reference lib="webworker" />

declare function importScripts(...urls: string[]): void

interface WorkboxModule {
  core: {
    clientsClaim(): void
  }
  strategies: {
    new NetworkOnly(): {
      handle(options: { request: Request }): Promise<Response>
    }
  }
  routing: {
    NavigationRoute: new (handler: unknown) => unknown
    registerRoute(route: unknown): void
  }
  precaching: {
    precacheAndRoute(entries: unknown): void
  }
  setConfig(config: { modulePathPrefix?: string; debug?: boolean }): void
  loadModule(name: string): void
}

declare const workbox: WorkboxModule

interface ServiceWorkerGlobalScope {
  __WB_MANIFEST: unknown
  skipWaiting(): Promise<void>
}

declare const self: ServiceWorkerGlobalScope
