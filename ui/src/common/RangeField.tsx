import React from 'react'
import PropTypes from 'prop-types'
import { useRecordContext } from 'react-admin'
import { formatRange } from './formatRange'

type RangeFieldProps = {
  className?: string
  source: string
  record?: Record<string, string | number | null | undefined>
  label?: string
}

export const RangeField = ({ className, source, ...rest }: RangeFieldProps) => {
  const record =
    useRecordContext<Record<string, string | number | null | undefined>>(rest)
  if (!record) return null
  return <span className={className}>{formatRange(record, source)}</span>
}

RangeField.propTypes = {
  label: PropTypes.string,
  record: PropTypes.object,
  source: PropTypes.string.isRequired,
}

RangeField.defaultProps = {
  addLabel: true,
}
