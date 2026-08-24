import { formatDuration } from '../utils'
import { useRecordContext } from 'react-admin'

type DurationFieldProps = {
  source: string
  label?: string
  record?: Record<string, unknown>
  addLabel?: boolean
}

export const DurationField = ({ source, ...rest }: DurationFieldProps) => {
  const record = useRecordContext<Record<string, unknown>>(rest)
  if (!record) return null
  try {
    return <span>{formatDuration(record[source])}</span>
  } catch (e) {
    // eslint-disable-next-line no-console
    console.log('Error in DurationField! Record:', record)
    return <span>00:00</span>
  }
}

DurationField.defaultProps = {
  addLabel: true,
}
