import type { Dispatch } from 'redux'
import throttle from './utils/throttle'
import { processEvent, serverDown, streamReconnected } from './actions'
import config from './config'
import { REST_URL } from './consts'
import { baseUrl } from './utils'

const RECONNECT_DELAY = 5000
const RECONNECT_JITTER = 1000

let eventStream: EventSource | null = null
let reconnectTimer: number | null = null

const newEventStream = async (): Promise<EventSource> => {
  let url = baseUrl(`${REST_URL}/events`)
  const token = localStorage.getItem('token')
  if (token) url += `?jwt=${token}`
  return new EventSource(url)
}

const eventHandler =
  (dispatchFn: Dispatch) =>
  (event: Event): void => {
    const message = event as MessageEvent<string>
    const data: unknown = JSON.parse(message.data)
    if (event.type !== 'keepAlive') {
      dispatchFn(processEvent(event.type, data))
    }
  }

const throttledEventHandler = (
  dispatchFn: Dispatch,
): ((event: Event) => void) => throttle(eventHandler(dispatchFn), 100)

const scheduleReconnect = (dispatchFn: Dispatch): void => {
  if (reconnectTimer !== null) return
  const jitter = Math.floor((Math.random() * 2 - 1) * RECONNECT_JITTER)
  reconnectTimer = window.setTimeout(() => {
    reconnectTimer = null
    void connect(dispatchFn)
  }, RECONNECT_DELAY + jitter)
}

const setupHandlers = (stream: EventSource, dispatchFn: Dispatch): void => {
  stream.addEventListener('serverStart', eventHandler(dispatchFn))
  stream.addEventListener('scanStatus', throttledEventHandler(dispatchFn))
  stream.addEventListener('refreshResource', eventHandler(dispatchFn))
  if (config.enableNowPlaying) {
    stream.addEventListener('nowPlayingCount', eventHandler(dispatchFn))
  }
  stream.addEventListener('keepAlive', eventHandler(dispatchFn))
  stream.onopen = () => dispatchFn(streamReconnected())
  stream.onerror = (event) => {
    // eslint-disable-next-line no-console
    console.log('EventStream error', event)
    dispatchFn(serverDown())
    stream.close()
    scheduleReconnect(dispatchFn)
  }
}

const connect = async (
  dispatchFn: Dispatch,
): Promise<EventSource | undefined> => {
  try {
    const stream = await newEventStream()
    eventStream = stream
    setupHandlers(stream, dispatchFn)
    return stream
  } catch (error) {
    // eslint-disable-next-line no-console
    console.log('Error connecting to server:', error)
    scheduleReconnect(dispatchFn)
    return undefined
  }
}

const startEventStreamLegacy = async (
  dispatchFn: Dispatch,
): Promise<EventSource | undefined> => {
  try {
    const stream = await newEventStream()
    stream.addEventListener('serverStart', eventHandler(dispatchFn))
    stream.addEventListener('scanStatus', throttledEventHandler(dispatchFn))
    stream.addEventListener('refreshResource', eventHandler(dispatchFn))
    if (config.enableNowPlaying) {
      stream.addEventListener('nowPlayingCount', eventHandler(dispatchFn))
    }
    stream.addEventListener('keepAlive', eventHandler(dispatchFn))
    stream.onopen = () => dispatchFn(streamReconnected())
    stream.onerror = (event) => {
      // eslint-disable-next-line no-console
      console.log('EventStream error', event)
      dispatchFn(serverDown())
    }
    return stream
  } catch (error) {
    // eslint-disable-next-line no-console
    console.log('Error connecting to server:', error)
    return undefined
  }
}

const startEventStreamNew = async (
  dispatchFn: Dispatch,
): Promise<EventSource | undefined> => {
  if (reconnectTimer !== null) {
    window.clearTimeout(reconnectTimer)
    reconnectTimer = null
  }
  if (eventStream) {
    eventStream.close()
    eventStream = null
  }
  return connect(dispatchFn)
}

export const startEventStream = async (
  dispatchFn: Dispatch,
): Promise<EventSource | undefined> => {
  if (!localStorage.getItem('is-authenticated')) return undefined
  return config.devNewEventStream
    ? startEventStreamNew(dispatchFn)
    : startEventStreamLegacy(dispatchFn)
}
