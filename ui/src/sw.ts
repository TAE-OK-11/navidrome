/* eslint-disable */
/// <reference path="./types/workbox.d.ts" />

import { createNavigationHandler } from './swNavigation'

const serviceWorker = self as unknown as ServiceWorkerGlobalScope

// documentation: https://developers.google.com/web/tools/workbox/modules/workbox-sw
importScripts('3rdparty/workbox/workbox-sw.js')

workbox.setConfig({
  modulePathPrefix: '3rdparty/workbox/',
  debug: false,
})

workbox.loadModule('workbox-core')
workbox.loadModule('workbox-strategies')
workbox.loadModule('workbox-routing')
workbox.loadModule('workbox-navigation-preload')
workbox.loadModule('workbox-precaching')

workbox.core.clientsClaim()
serviceWorker.skipWaiting()

addEventListener('message', (event: MessageEvent) => {
  if (event.data && event.data.type === 'SKIP_WAITING') {
    serviceWorker.skipWaiting()
  }
})

const CACHE_NAME = 'offline-html'
// This assumes /offline.html is a URL for your self-contained
// (no external images or styles) offline page.
const FALLBACK_HTML_URL = './offline.html'
// Populate the cache with the offline HTML page when the
// service worker is installed.
serviceWorker.addEventListener('install', (event: ExtendableEvent) => {
  event.waitUntil(
    caches.open(CACHE_NAME).then((cache) => cache.add(FALLBACK_HTML_URL)),
  )
})

const networkOnly = new workbox.strategies.NetworkOnly()
const fallbackToCachedHtml = () =>
  caches.match(FALLBACK_HTML_URL, { cacheName: CACHE_NAME })

const navigationHandler = createNavigationHandler(
  networkOnly,
  fallbackToCachedHtml,
)

// self.__WB_MANIFEST is default injection point
workbox.precaching.precacheAndRoute(self.__WB_MANIFEST)

// Register this strategy to handle all navigations.
workbox.routing.registerRoute(
  new workbox.routing.NavigationRoute(navigationHandler),
)
