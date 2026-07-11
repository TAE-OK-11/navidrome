import { beforeEach, describe, expect, it, vi } from 'vitest'
import httpClient from '../dataProvider/httpClient'
import {
  getHotCacheDashboard,
  normalizeCandidates,
  normalizeDashboard,
  normalizePage,
  promoteHotCacheCandidates,
  queryString,
} from './api'

vi.mock('../dataProvider/httpClient', () => ({ default: vi.fn() }))

describe('Hot Cache API', () => {
  beforeEach(() => httpClient.mockReset())

  it('accepts null and undefined query objects', () => {
    expect(queryString(null)).toBe('')
    expect(queryString(undefined)).toBe('')
    expect(queryString({ search: 'song', offset: 0, empty: null })).toBe(
      '?search=song&offset=0',
    )
  })

  it('normalizes missing dashboard collections', () => {
    expect(normalizeDashboard(null)).toEqual({
      status: {},
      sessions: [],
      queue: [],
      current: null,
      formats: [],
      events: [],
      errors: [],
      artwork: [],
    })
    expect(normalizePage(null)).toEqual({ items: [], total: 0 })
    expect(normalizeCandidates({ items: null })).toEqual({
      items: [],
      hasMore: false,
    })
  })

  it('loads the dashboard with one HTTP request', async () => {
    httpClient.mockResolvedValue({ json: { status: { enabled: true } } })
    const dashboard = await getHotCacheDashboard()
    expect(httpClient).toHaveBeenCalledTimes(1)
    expect(httpClient.mock.calls[0][0]).toContain('/dashboard?eventLimit=200')
    expect(dashboard.status.enabled).toBe(true)
    expect(dashboard.sessions).toEqual([])
  })

  it('coalesces concurrent dashboard refreshes', async () => {
    let resolve
    httpClient.mockReturnValue(
      new Promise((done) => {
        resolve = done
      }),
    )
    const first = getHotCacheDashboard()
    const second = getHotCacheDashboard()
    expect(httpClient).toHaveBeenCalledTimes(1)
    resolve({ json: { status: { health: 'healthy' } } })
    await expect(first).resolves.toEqual(await second)
  })

  it('sends selected media IDs in one promotion request', async () => {
    httpClient.mockResolvedValue({
      json: { accepted: ['one'], rejected: null },
    })
    const result = await promoteHotCacheCandidates(['one'])
    const options = httpClient.mock.calls[0][1]
    expect(options.method).toBe('POST')
    expect(JSON.parse(options.body)).toEqual({ mediaIds: ['one'] })
    expect(result).toEqual({ accepted: ['one'], rejected: {} })
  })
})
