import { describe, expect, it } from 'vitest'
import { formatDurationNs, formatMicros, formatRate } from './formatters'

describe('Hot Cache formatters', () => {
  it('formats rates and Go durations', () => {
    expect(formatRate(0.754)).toBe('75.4%')
    expect(formatDurationNs(2_500_000)).toBe('3 ms')
    expect(formatDurationNs(2_500_000_000)).toBe('2.5 s')
  })

  it('formats histogram microseconds', () => {
    expect(formatMicros(750)).toBe('750 us')
    expect(formatMicros(2500)).toBe('2.50 ms')
  })
})
