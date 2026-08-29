import {
  EVENT_REFRESH_RESOURCE,
  EVENT_SCAN_STATUS,
  EVENT_SERVER_START,
  EVENT_NOW_PLAYING_COUNT,
  EVENT_NOW_PLAYING_COUNT_SYNC,
  EVENT_STREAM_RECONNECTED,
} from '../actions'
import config from '../config'
import type { ActivityState, ScanStatus, UnknownAction } from '../types/redux'

const initialState: ActivityState = {
  scanStatus: {
    scanning: false,
    folderCount: 0,
    count: 0,
    error: '',
    elapsedTime: 0,
  },
  serverStart: { version: config.version },
  nowPlayingCount: 0,
  nowPlayingLastUpdate: 0,
  streamReconnected: 0, // Timestamp of last reconnection
}

export const activityReducer = (
  previousState: ActivityState = initialState,
  payload: UnknownAction,
): ActivityState => {
  const { type, data } = payload

  switch (type) {
    case EVENT_SCAN_STATUS: {
      const scanData = (data ?? {}) as ScanStatus
      const elapsedTime = Number(scanData.elapsedTime) || 0
      return { ...previousState, scanStatus: { ...scanData, elapsedTime } }
    }
    case EVENT_SERVER_START: {
      const serverData = (data ?? {}) as {
        startTime?: string
        version?: string
      }
      return {
        ...previousState,
        serverStart: {
          startTime: serverData.startTime
            ? Date.parse(serverData.startTime)
            : undefined,
          version: serverData.version ?? previousState.serverStart.version,
        },
      }
    }
    case EVENT_REFRESH_RESOURCE:
      return {
        ...previousState,
        refresh: {
          lastReceived: Date.now(),
          resources: data,
        },
      }
    case EVENT_NOW_PLAYING_COUNT: {
      const countData = (data ?? {}) as { count?: number }
      return {
        ...previousState,
        nowPlayingCount: countData.count ?? 0,
        nowPlayingLastUpdate: Date.now(),
      }
    }
    case EVENT_NOW_PLAYING_COUNT_SYNC: {
      const countData = (data ?? {}) as { count?: number }
      return { ...previousState, nowPlayingCount: countData.count ?? 0 }
    }
    case EVENT_STREAM_RECONNECTED:
      return { ...previousState, streamReconnected: Date.now() }
    default:
      return previousState
  }
}
