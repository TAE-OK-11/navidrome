import { useState, useCallback, useEffect, useRef } from 'react'
import { useDataProvider, useNotify, useRefresh } from 'react-admin'
import subsonic from '../subsonic'
import type { Identifier } from 'react-admin'

type RatingRecord = {
  id?: Identifier
  rating?: number
  mediaFileId?: Identifier
  playlistId?: Identifier
}

export const useRating = (
  resource: string,
  record: RatingRecord,
): readonly [
  (val: number, id: Identifier) => Promise<void>,
  number,
  boolean,
] => {
  const [loading, setLoading] = useState(false)
  const [rating, setRating] = useState(record.rating || 0)
  const notify = useNotify()
  const refresh = useRefresh()
  const dataProvider = useDataProvider()
  const mountedRef = useRef(false)

  useEffect(() => {
    mountedRef.current = true
    return () => {
      mountedRef.current = false
    }
  }, [])

  useEffect(() => {
    setRating(record.rating || 0)
  }, [record.id, record.rating])

  const refreshRating = useCallback(() => {
    // For playlist tracks, refresh both resources to keep data in sync
    if (record.mediaFileId) {
      // This is a playlist track - refresh both the playlist track and the song
      const promises = [
        dataProvider.getOne('song', { id: record.mediaFileId }),
        dataProvider.getOne('playlistTrack', {
          id: record.id,
          filter: { playlist_id: record.playlistId },
        }),
      ]

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
    } else {
      // Regular song or other resource
      return dataProvider
        .getOne(resource, { id: record.id })
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
    }
  }, [
    dataProvider,
    record.id,
    record.mediaFileId,
    record.playlistId,
    refresh,
    resource,
  ])

  const rate = (val: number, id: Identifier): Promise<void> => {
    if (loading || !id) return Promise.resolve()

    const previousRating = rating
    setLoading(true)
    setRating(val)
    return subsonic
      .setRating(String(id), val)
      .then(() => {
        void refreshRating()
      })
      .catch((e) => {
        // eslint-disable-next-line no-console
        console.log('Error setting star rating: ', e)
        setRating(previousRating)
        notify('ra.page.error', { type: 'warning' })
        if (mountedRef.current) {
          setLoading(false)
        }
      })
      .then(() => undefined)
  }

  return [rate, rating, loading] as const
}
