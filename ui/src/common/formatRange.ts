type RangeRecord = Record<string, string | number | null | undefined>

export const formatRange = (record: RangeRecord, source: string) => {
  const nameCapitalized = source.charAt(0).toUpperCase() + source.slice(1)
  const min = record[`min${nameCapitalized}`]
  const max = record[`max${nameCapitalized}`]
  const range: Array<string | number> = []
  if (min) {
    range.push(min)
  }
  if (max && max !== min) {
    range.push(max)
  }
  return range.join('-')
}
