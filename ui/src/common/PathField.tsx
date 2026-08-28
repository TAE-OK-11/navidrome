import React from 'react'
import { usePermissions, useRecordContext } from 'react-admin'
import config from '../config'

type PathRecord = {
  libraryPath?: string
  path?: string
}

type PathFieldProps = {
  record?: PathRecord
  source?: string
  sortable?: boolean
}

export const PathField = (props: PathFieldProps) => {
  const record = useRecordContext<PathRecord>(props)
  const { permissions } = usePermissions()
  if (!record) return null
  let path = permissions === 'admin' ? record.libraryPath : ''
  const recordPath = record.path ?? ''

  if (path && path.endsWith(config.separator)) {
    path = `${path}${recordPath}`
  } else {
    path = path ? `${path}${config.separator}${recordPath}` : recordPath
  }

  return <span>{path}</span>
}
