// @ts-nocheck -- legacy JavaScript migration; remove after typing this module
import { useCallback, useEffect, useRef, useState } from 'react'
import { useDataProvider, useNotify, useRefresh } from 'react-admin'
import subsonic from '../subsonic'

export const useToggleLove = (resource, record = {}) => {
  const [loading, setLoading] = useState(false)
  const [loved, setLoved] = useState(Boolean(record.starred))
  const notify = useNotify()
  const refresh = useRefresh()

  const mountedRef = useRef(false)
  useEffect(() => {
    mountedRef.current = true
    return () => {
      mountedRef.current = false
    }
  }, [])

  useEffect(() => {
    setLoved(Boolean(record.starred))
  }, [record.id, record.starred])

  const dataProvider = useDataProvider()

  const refreshRecord = useCallback(() => {
    const promises = []

    // Always refresh the original resource
    const params = { id: record.id }
    if (record.playlistId) {
      params.filter = { playlist_id: record.playlistId }
    }
    promises.push(dataProvider.getOne(resource, params))

    // If we have a mediaFileId, also refresh the song
    if (record.mediaFileId) {
      promises.push(dataProvider.getOne('song', { id: record.mediaFileId }))
    }

    return Promise.all(promises)
      .catch((e) => {
        // eslint-disable-next-line no-console
        console.log('Error encountered: ' + e)
      })
      .finally(() => {
        refresh()
        if (mountedRef.current) {
          setLoading(false)
        }
      })
  }, [
    dataProvider,
    record.mediaFileId,
    record.id,
    record.playlistId,
    refresh,
    resource,
  ])

  const toggleLove = () => {
    if (loading || (!record.id && !record.mediaFileId)) return Promise.resolve()

    const previousLoved = loved
    const toggle = previousLoved ? subsonic.unstar : subsonic.star
    const id = record.mediaFileId || record.id

    setLoading(true)
    setLoved(!previousLoved)
    return toggle(id)
      .then(refreshRecord)
      .catch((e) => {
        // eslint-disable-next-line no-console
        console.log('Error toggling love: ', e)
        setLoved(previousLoved)
        notify('ra.page.error', { type: 'warning' })
        if (mountedRef.current) {
          setLoading(false)
        }
      })
  }

  return [toggleLove, loading, loved]
}
