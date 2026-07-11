import { formatBytes } from '../utils'

export const formatRate = (value) => `${((value || 0) * 100).toFixed(1)}%`

export const formatDurationNs = (nanoseconds) => {
  const seconds = Math.max(0, Number(nanoseconds || 0) / 1e9)
  if (seconds < 1) return `${Math.round(seconds * 1000)} ms`
  if (seconds < 60) return `${seconds.toFixed(1)} s`
  const minutes = Math.floor(seconds / 60)
  return `${minutes}m ${Math.round(seconds % 60)}s`
}

export const formatDate = (value) =>
  value ? new Date(value).toLocaleString() : '-'

export const formatStorage = (value) => formatBytes(Number(value || 0))

export const formatNumber = (value) => Number(value || 0).toLocaleString()

export const formatMicros = (value) => {
  const micros = Number(value || 0)
  return micros >= 1000 ? `${(micros / 1000).toFixed(2)} ms` : `${micros} us`
}
