import { Children, cloneElement, isValidElement } from 'react'
import { isWritable } from './playlistUtils'
import { useRecordContext } from 'react-admin'

export const Writable = (props) => {
  const { children } = props
  const record = useRecordContext(props) || {}
  if (isWritable(record.ownerId)) {
    return Children.map(children, (child) =>
      isValidElement(child) ? cloneElement(child, props) : child,
    )
  }
  return null
}
