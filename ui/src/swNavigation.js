// Lives outside sw.js so it can be unit tested: sw.js only loads inside a
// service worker, where Workbox arrives through importScripts.
export const createNavigationHandler =
  (networkOnly, offlineFallback) => async (params) => {
    try {
      const response = await networkOnly.handle(params)
      // Workbox treats a 5xx response as a successful fetch, but it is not a
      // usable application shell. Serve the cached offline page instead.
      return response.status >= 500 ? offlineFallback() : response
    } catch (error) {
      return offlineFallback()
    }
  }
