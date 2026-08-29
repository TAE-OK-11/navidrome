import { formatDuration } from '../utils'
import { useRecordContext } from 'react-admin'

type DurationFieldProps = {
  source: string
  label?: string
  record?: Record<string, unknown>
  addLabel?: boolean
  sortable?: boolean
  sortByOrder?: string
}

export const DurationField = ({ source, ...rest }: DurationFieldProps) => {
  const record = useRecordContext<Record<string, unknown>>(rest)
  if (!record) return null
  try {
    const duration = record[source]
    return (
      <span>{formatDuration(typeof duration === 'number' ? duration : 0)}</span>
    )
  } catch (e) {
    // eslint-disable-next-line no-console
    console.log('Error in DurationField! Record:', record)
    return <span>00:00</span>
  }
}

DurationField.defaultProps = {
  addLabel: true,
}
