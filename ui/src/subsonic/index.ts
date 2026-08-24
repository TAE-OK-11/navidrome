import { baseUrl } from '../utils'
import {
  httpClient,
  clientUniqueId,
  clientUniqueIdHeader,
} from '../dataProvider'

type QueryPrimitive = string | number | boolean
type QueryValue = QueryPrimitive | null | undefined | QueryPrimitive[]
type QueryOptions = Record<string, QueryValue>

type CoverArtRecord = {
  id: string | number
  updatedAt?: QueryPrimitive
  album?: unknown
  albumArtist?: unknown
  sync?: unknown
  streamUrl?: unknown
}

const url = (
  command: string,
  id?: string | number | null,
  options?: QueryOptions | null,
): string => {
  const username = localStorage.getItem('username')
  const token = localStorage.getItem('subsonic-token')
  const salt = localStorage.getItem('subsonic-salt')
  if (!username || !token || !salt) {
    return ''
  }

  const params = new URLSearchParams()
  params.append('u', username)
  params.append('t', token)
  params.append('s', salt)
  params.append('f', 'json')
  params.append('v', '1.8.0')
  params.append('c', 'NavidromeUI')
  if (id != null && id !== '') params.append('id', String(id))
  if (options) {
    Object.entries(options).forEach(([key, optionValue]) => {
      if (key === 'ts') {
        if (optionValue) params.append('_', String(Date.now()))
        return
      }
      // Handle array parameters by appending each value separately
      if (Array.isArray(optionValue)) {
        optionValue.forEach((value) => params.append(key, String(value)))
      } else if (optionValue != null) {
        params.append(key, String(optionValue))
      }
    })
  }
  return `/rest/${command}?${params.toString()}`
}

const ping = () => httpClient(url('ping'))

const reportPlaybackUrl = (
  mediaId: string,
  positionMs: number,
  state: string,
): string =>
  url('reportPlayback', null, { mediaId, mediaType: 'song', positionMs, state })

const reportPlayback = (mediaId: string, positionMs: number, state: string) =>
  httpClient(reportPlaybackUrl(mediaId, positionMs, state))

const reportPlaybackKeepalive = (
  mediaId: string,
  positionMs: number,
  state: string,
): void => {
  const u = reportPlaybackUrl(mediaId, positionMs, state)
  if (u) {
    fetch(baseUrl(u), {
      keepalive: true,
      headers: { [clientUniqueIdHeader]: clientUniqueId },
    })
  }
}

const star = (id: string) => httpClient(url('star', id))

const unstar = (id: string) => httpClient(url('unstar', id))

const setRating = (id: string, rating: number) =>
  httpClient(url('setRating', id, { rating }))

const download = (id: string, format = 'raw', bitrate = '0'): string =>
  (window.location.href = baseUrl(url('download', id, { format, bitrate })))

const startScan = (options: QueryOptions) =>
  httpClient(url('startScan', null, options))

const getScanStatus = () => httpClient(url('getScanStatus'))

const getNowPlaying = () => httpClient(url('getNowPlaying'))

const getAvatarUrl = (username: string, size?: number): string =>
  baseUrl(
    url('getAvatar', null, {
      username,
      ...(size && { size }),
    }),
  )

const getCoverArtUrl = (
  record: CoverArtRecord,
  size?: number,
  square?: boolean,
): string => {
  const options: QueryOptions = {
    ...(record.updatedAt && { _: record.updatedAt }),
    ...(size && { size }),
    ...(square && { square }),
  }

  // TODO Move this logic to server
  if (record.album) {
    return baseUrl(url('getCoverArt', 'mf-' + record.id, options))
  } else if (record.albumArtist) {
    return baseUrl(url('getCoverArt', 'al-' + record.id, options))
  } else if (record.sync !== undefined) {
    // This is a playlist
    return baseUrl(url('getCoverArt', 'pl-' + record.id, options))
  } else if (record.streamUrl !== undefined) {
    // This is a radio station
    return baseUrl(url('getCoverArt', 'ra-' + record.id, options))
  } else {
    return baseUrl(url('getCoverArt', 'ar-' + record.id, options))
  }
}

const getDiscCoverArtUrl = (
  albumId: string,
  discNumber: number,
  updatedAt?: QueryPrimitive,
  size?: number,
): string => {
  const options: QueryOptions = {
    ...(updatedAt && { _: updatedAt }),
    ...(size && { size }),
  }
  return baseUrl(
    url('getCoverArt', 'dc-' + albumId + ':' + discNumber, options),
  )
}

const getArtistInfo = (id: string) => {
  return httpClient(url('getArtistInfo', id))
}

const getAlbumInfo = (id: string) => {
  return httpClient(url('getAlbumInfo', id))
}

const getSimilarSongs2 = (id: string, count = 100) => {
  return httpClient(url('getSimilarSongs2', id, { count }))
}

const getTopSongs = (artist: string, count = 50) => {
  return httpClient(url('getTopSongs', null, { artist, count }))
}

const streamUrl = (id: string, options?: QueryOptions): string => {
  return baseUrl(
    url('stream', id, {
      ts: true,
      ...options,
    }),
  )
}

export default {
  url,
  ping,
  reportPlayback,
  reportPlaybackKeepalive,
  download,
  star,
  unstar,
  setRating,
  startScan,
  getScanStatus,
  getNowPlaying,
  getCoverArtUrl,
  getDiscCoverArtUrl,
  getAvatarUrl,
  streamUrl,
  getAlbumInfo,
  getArtistInfo,
  getTopSongs,
  getSimilarSongs2,
}
