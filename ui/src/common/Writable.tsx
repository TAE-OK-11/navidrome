import { Children, cloneElement, isValidElement, type ReactNode } from 'react'
import { isWritable } from './playlistUtils'
import { useRecordContext } from 'react-admin'
import type { PlaylistRecord } from '../types/records'

export const Writable = (props: { children?: ReactNode }) => {
  const { children } = props
  const record = (useRecordContext<PlaylistRecord>(props) || {}) as PlaylistRecord
  if (isWritable(record.ownerId)) {
    return Children.map(children, (child) =>
      isValidElement(child) ? cloneElement(child, props) : child,
    )
  }
  return null
}
