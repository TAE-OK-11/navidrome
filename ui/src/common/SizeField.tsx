import { formatBytes } from '../utils'
import { useRecordContext } from 'react-admin'
import { Box } from '@mui/material'

type SizeFieldProps = {
  source: string
  label?: string
  record?: Record<string, unknown>
  addLabel?: boolean
  sortable?: boolean
  sortByOrder?: string
}

export const SizeField = ({ source, ...rest }: SizeFieldProps) => {
  const record = useRecordContext<Record<string, unknown>>(rest)
  if (!record) return null
  const size = record[source]
  return (
    <Box component="span" sx={{ display: 'inline-block' }}>
      {typeof size === 'number' && size > 0 ? formatBytes(size) : '0 MB'}
    </Box>
  )
}

SizeField.defaultProps = {
  addLabel: true,
}
