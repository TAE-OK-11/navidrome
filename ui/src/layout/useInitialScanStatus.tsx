import { useEffect } from 'react'
import { useDispatch } from 'react-redux'
import subsonic from '../subsonic'
import { scanStatusUpdate } from '../actions'

export const useInitialScanStatus = () => {
  const dispatch = useDispatch()
  useEffect(() => {
    subsonic
      .getScanStatus()
      .then((resp) => resp.json['subsonic-response'])
      .then((data) => {
        if (data?.status === 'ok') {
          dispatch(scanStatusUpdate(data.scanStatus))
        }
      })
      .catch((error) => {
        // Scan status is supplemental UI state and must not break app startup.
        // eslint-disable-next-line no-console
        console.warn('Could not load initial scan status:', error)
      })
  }, [dispatch])
}
