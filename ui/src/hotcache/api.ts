import httpClient from '../dataProvider/httpClient'

const ROOT = '/api/admin/hot-cache'
const DASHBOARD_TIMEOUT_MS = 10000

type JsonObject = Record<string, unknown>
export type HotCacheQuery = Record<string, unknown>

export interface HotCacheDashboard {
  status: JsonObject
  sessions: unknown[]
  queue: unknown[]
  current: unknown | null
  formats: unknown[]
  events: unknown[]
  errors: unknown[]
  artwork: unknown[]
}

export interface HotCachePage<T = unknown> {
  items: T[]
  total: number
}

export interface HotCacheCandidates<T = unknown> {
  items: T[]
  hasMore: boolean
}

export interface HotCachePromotionResult {
  accepted: unknown[]
  rejected: JsonObject
}

let dashboardRequest: Promise<HotCacheDashboard> | null = null

const asObject = (value: unknown): JsonObject =>
  value && typeof value === 'object' && !Array.isArray(value)
    ? (value as JsonObject)
    : {}

const asArray = <T = unknown>(value: unknown): T[] =>
  Array.isArray(value) ? (value as T[]) : []

export const queryString = (query?: HotCacheQuery): string => {
  const params = new URLSearchParams()
  Object.entries(asObject(query)).forEach(([key, value]) => {
    if (value !== undefined && value !== null && value !== '') {
      params.set(key, String(value))
    }
  })
  const value = params.toString()
  return value ? `?${value}` : ''
}

export const normalizeDashboard = (value: unknown): HotCacheDashboard => {
  const dashboard = asObject(value)
  return {
    status: asObject(dashboard.status),
    sessions: asArray(dashboard.sessions),
    queue: asArray(dashboard.queue),
    current: dashboard.current ?? null,
    formats: asArray(dashboard.formats),
    events: asArray(dashboard.events),
    errors: asArray(dashboard.errors),
    artwork: asArray(dashboard.artwork),
  }
}

export const normalizePage = <T = unknown>(value: unknown): HotCachePage<T> => {
  const page = asObject(value)
  return { items: asArray<T>(page.items), total: Number(page.total) || 0 }
}

export const normalizeCandidates = <T = unknown>(
  value: unknown,
): HotCacheCandidates<T> => {
  const page = asObject(value)
  return { items: asArray<T>(page.items), hasMore: Boolean(page.hasMore) }
}

export const getHotCache = <T = unknown>(
  path: string,
  query?: HotCacheQuery,
  signal?: AbortSignal,
): Promise<T> =>
  httpClient(`${ROOT}/${path}${queryString(query)}`, { signal }).then(
    ({ json }: { json: T }) => json,
  )

export const hotCacheAction = <T = unknown>(
  path: string,
  method = 'POST',
  headers?: Record<string, string>,
): Promise<T> =>
  httpClient(`${ROOT}/${path}`, {
    method,
    headers: new Headers({ Accept: 'application/json', ...headers }),
  }).then(({ json }: { json: T }) => json)

export const getHotCacheEntries = <T = unknown>(
  query?: HotCacheQuery,
  signal?: AbortSignal,
): Promise<HotCachePage<T>> =>
  getHotCache('entries', query, signal).then((value) => normalizePage<T>(value))

export const getHotCacheDashboard = (): Promise<HotCacheDashboard> => {
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

export const getHotCacheCandidates = <T = unknown>(
  query?: HotCacheQuery,
  signal?: AbortSignal,
): Promise<HotCacheCandidates<T>> =>
  getHotCache('candidates', query, signal).then((value) =>
    normalizeCandidates<T>(value),
  )

export const promoteHotCacheCandidates = (
  mediaIds: unknown,
): Promise<HotCachePromotionResult> =>
  httpClient(`${ROOT}/promote`, {
    method: 'POST',
    headers: new Headers({
      Accept: 'application/json',
      'Content-Type': 'application/json',
    }),
    body: JSON.stringify({ mediaIds: asArray(mediaIds) }),
  }).then(({ json }: { json?: JsonObject }) => ({
    accepted: asArray(json?.accepted),
    rejected: asObject(json?.rejected),
  }))
