import React from 'react'
import {
  Button,
  useDataProvider,
  useTranslate,
  useUnselectAll,
  useNotify,
} from 'react-admin'
import { useDispatch } from 'react-redux'

export const BatchPlayButton = ({
  resource,
  selectedIds,
  action,
  label,
  icon,
  className,
  sx,
}) => {
  const dispatch = useDispatch()
  const translate = useTranslate()
  const dataProvider = useDataProvider()
  const unselectAll = useUnselectAll()
  const notify = useNotify()

  const addToQueue = () => {
    dataProvider
      .getMany(resource, { ids: selectedIds })
      .then((response) => {
        // Add tracks to a map for easy lookup by ID, needed for the next step
        const tracks = {}
        for (const track of response.data) tracks[track.id] = track
        // Add the tracks to the queue in the selection order
        dispatch(action(tracks, selectedIds))
      })
      .catch(() => {
        notify('ra.page.error', { type: 'warning' })
      })
    unselectAll(resource)
  }

  const caption = translate(label)
  return (
    <Button
      aria-label={caption}
      onClick={addToQueue}
      label={caption}
      className={className}
      sx={sx}
    >
      {icon}
    </Button>
  )
}
