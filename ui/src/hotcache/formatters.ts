import { formatBytes } from '../utils'

const numeric = (value: unknown): number => Number(value ?? 0) || 0

export const formatRate = (value: unknown): string =>
  `${(numeric(value) * 100).toFixed(1)}%`

export const formatDurationNs = (nanoseconds: unknown): string => {
  const seconds = Math.max(0, numeric(nanoseconds) / 1e9)
  if (seconds < 1) return `${Math.round(seconds * 1000)} ms`
  if (seconds < 60) return `${seconds.toFixed(1)} s`
  const minutes = Math.floor(seconds / 60)
  return `${minutes}m ${Math.round(seconds % 60)}s`
}

export const formatDate = (
  value: string | number | Date | null | undefined,
): string => (value ? new Date(value).toLocaleString() : '-')

export const formatStorage = (value: unknown): string =>
  formatBytes(numeric(value))

export const formatNumber = (value: unknown): string =>
  numeric(value).toLocaleString()

export const formatMicros = (value: unknown): string => {
  const micros = numeric(value)
  return micros >= 1000 ? `${(micros / 1000).toFixed(2)} ms` : `${micros} us`
}
