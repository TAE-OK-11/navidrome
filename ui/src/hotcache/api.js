import httpClient from '../dataProvider/httpClient'

const ROOT = '/api/admin/hot-cache'
const DASHBOARD_TIMEOUT_MS = 10000
let dashboardRequest = null

const asObject = (value) =>
  value && typeof value === 'object' && !Array.isArray(value) ? value : {}

const asArray = (value) => (Array.isArray(value) ? value : [])

export const queryString = (query) => {
  const params = new URLSearchParams()
  Object.entries(asObject(query)).forEach(([key, value]) => {
    if (value !== undefined && value !== null && value !== '') {
      params.set(key, String(value))
    }
  })
  const value = params.toString()
  return value ? `?${value}` : ''
}

export const normalizeDashboard = (value) => {
  const dashboard = asObject(value)
  return {
    status: asObject(dashboard.status),
    sessions: asArray(dashboard.sessions),
    queue: asArray(dashboard.queue),
    current: dashboard.current || null,
    formats: asArray(dashboard.formats),
    events: asArray(dashboard.events),
    errors: asArray(dashboard.errors),
    artwork: asArray(dashboard.artwork),
  }
}

export const normalizePage = (value) => {
  const page = asObject(value)
  return { items: asArray(page.items), total: Number(page.total) || 0 }
}

export const normalizeCandidates = (value) => {
  const page = asObject(value)
  return { items: asArray(page.items), hasMore: Boolean(page.hasMore) }
}

export const getHotCache = (path, query, signal) =>
  httpClient(`${ROOT}/${path}${queryString(query)}`, { signal }).then(
    ({ json }) => json,
  )

export const hotCacheAction = (path, method = 'POST', headers) =>
  httpClient(`${ROOT}/${path}`, {
    method,
    headers: new Headers({ Accept: 'application/json', ...headers }),
  }).then(({ json }) => json)

export const getHotCacheEntries = (query, signal) =>
  getHotCache('entries', query, signal).then(normalizePage)

export const getHotCacheDashboard = () => {
  if (dashboardRequest) return dashboardRequest

  const controller = new AbortController()
  const timeout = window.setTimeout(
    () => controller.abort(),
    DASHBOARD_TIMEOUT_MS,
  )
  dashboardRequest = getHotCache(
    'dashboard',
    { eventLimit: 200 },
    controller.signal,
  )
    .then(normalizeDashboard)
    .finally(() => {
      window.clearTimeout(timeout)
      dashboardRequest = null
    })
  return dashboardRequest
}

export const getHotCacheCandidates = (query, signal) =>
  getHotCache('candidates', query, signal).then(normalizeCandidates)

export const promoteHotCacheCandidates = (mediaIds) =>
  httpClient(`${ROOT}/promote`, {
    method: 'POST',
    headers: new Headers({
      Accept: 'application/json',
      'Content-Type': 'application/json',
    }),
    body: JSON.stringify({ mediaIds: asArray(mediaIds) }),
  }).then(({ json }) => ({
    accepted: asArray(json?.accepted),
    rejected: asObject(json?.rejected),
  }))
