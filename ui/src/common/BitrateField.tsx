import { useRecordContext } from 'react-admin'

type BitrateFieldProps = {
  source: string
  label?: string
  record?: Record<string, unknown>
  addLabel?: boolean
}

export const BitrateField = ({ source, ...rest }: BitrateFieldProps) => {
  const record = useRecordContext<Record<string, unknown>>(rest)
  return record ? <span>{`${record[source]} kbps`}</span> : null
}

BitrateField.defaultProps = {
  addLabel: true,
}
