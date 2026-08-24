import React from 'react'
import { isDateSet } from '../utils/validations'
import { DateField as RADateField, useRecordContext } from 'react-admin'

export const DateField = (props) => {
  const { source } = props
  const record = useRecordContext(props)
  const value = record?.[source]
  if (!isDateSet(value)) return null
  return <RADateField {...props} />
}

DateField.defaultProps = {
  addLabel: true,
}
