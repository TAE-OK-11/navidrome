import { describe, it, beforeEach, afterEach, vi, expect } from 'vitest'
import { startEventStream } from './eventStream'
import { serverDown, streamReconnected } from './actions'
import config from './config'

class MockEventSource {
  url: string
  readyState: number
  listeners: Record<string, (event: Event) => void>
  onerror: ((event: Event) => void) | null
  onopen: ((event: Event) => void) | null

  constructor(url: string) {
    this.url = url
    this.readyState = 1
    this.listeners = {}
    this.onerror = null
    this.onopen = null
  }

  addEventListener(type: string, handler: (event: Event) => void) {
    this.listeners[type] = handler
  }

  close() {
    this.readyState = 2
  }
}

describe('startEventStream', () => {
  vi.useFakeTimers()
  let dispatch: ReturnType<typeof vi.fn>
  let instance: MockEventSource

  beforeEach(() => {
    dispatch = vi.fn()
    globalThis.EventSource = vi.fn().mockImplementation(function (url: string) {
      instance = new MockEventSource(url)
      return instance
    }) as unknown as typeof EventSource
    localStorage.setItem('is-authenticated', 'true')
    localStorage.setItem('token', 'abc')
    config.devNewEventStream = true
    vi.spyOn(Math, 'random').mockReturnValue(0.5)
    vi.spyOn(console, 'log').mockImplementation(() => {})
  })

  afterEach(() => {
    config.devNewEventStream = false
    vi.restoreAllMocks()
  })

  it('marks the stream reconnected only after it actually opens', async () => {
    await startEventStream(dispatch as any)
    expect(dispatch).not.toHaveBeenCalledWith(streamReconnected())

    instance.onopen?.(new Event('open'))
    expect(dispatch).toHaveBeenCalledWith(streamReconnected())
  })

  it('reconnects after an error', async () => {
    await startEventStream(dispatch as any)
    expect(globalThis.EventSource).toHaveBeenCalledTimes(1)
    instance.onerror?.(new Event('error'))
    expect(dispatch).toHaveBeenCalledWith(serverDown())
    vi.advanceTimersByTime(5000)
    expect(globalThis.EventSource).toHaveBeenCalledTimes(2)
  })
})
