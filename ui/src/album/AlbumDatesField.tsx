import { useRecordContext } from 'react-admin'
import { formatRange } from '../common/index'

const originalYearSymbol = '♫'
const releaseYearSymbol = '○'

type AlbumDatesRecord = Record<string, string | number | null | undefined> & {
  releaseDate?: string | number
  maxYear?: string | number
}

type AlbumDatesFieldProps = {
  className?: string
  record?: AlbumDatesRecord
}

export const AlbumDatesField = ({
  className,
  ...rest
}: AlbumDatesFieldProps) => {
  const record = useRecordContext<AlbumDatesRecord>(rest)
  if (!record) return null
  const releaseDate = record.releaseDate
  const releaseYear = releaseDate?.toString().substring(0, 4)
  const yearRange =
    formatRange(record, 'originalYear') || record.maxYear?.toString()

  // Don't show anything if the year starts with "0"
  if (yearRange === '0' || releaseYear?.startsWith('0')) {
    return null
  }

  let label = yearRange

  if (releaseYear !== undefined && yearRange !== releaseYear) {
    label = `${originalYearSymbol} ${yearRange} · ${releaseYearSymbol} ${releaseYear}`
  }
  return <span className={className}>{label}</span>
}
