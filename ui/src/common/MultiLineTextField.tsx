import React, { memo } from 'react'
import Typography from '@mui/material/Typography'
import sanitizeFieldRestProps from './sanitizeFieldRestProps'
import md5 from 'blueimp-md5'
import { useRecordContext } from 'react-admin'

type MultiLineTextFieldProps = {
  className?: string
  emptyText?: React.ReactNode
  source: string
  firstLine?: number
  maxLines?: number
  addLabel?: boolean
  record?: Record<string, unknown>
}

export const MultiLineTextField = memo(
  ({
    className,
    emptyText,
    source,
    firstLine = 0,
    maxLines,
    addLabel,
    ...rest
  }: MultiLineTextFieldProps) => {
    const record = useRecordContext<Record<string, unknown>>(rest)
    const value = record && record[source]
    let lines = typeof value === 'string' ? value.split('\n') : []
    if (maxLines || firstLine) {
      lines = lines.slice(firstLine, maxLines)
    }

    return (
      <Typography
        className={className}
        variant="body2"
        component="span"
        {...sanitizeFieldRestProps(rest)}
      >
        {lines.length === 0 && emptyText ? emptyText : lines}
      </Typography>
    )
  },
)

MultiLineTextField.displayName = 'MultiLineTextField'

MultiLineTextField.defaultProps = { addLabel: true }
