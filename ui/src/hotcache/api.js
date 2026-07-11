import httpClient from '../dataProvider/httpClient'

const ROOT = '/api/admin/hot-cache'

const queryString = (query = {}) => {
  const params = new URLSearchParams()
  Object.entries(query).forEach(([key, value]) => {
    if (value !== undefined && value !== null && value !== '') {
      params.set(key, String(value))
    }
  })
  const value = params.toString()
  return value ? `?${value}` : ''
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
  getHotCache('entries', query, signal)
