import React, { useState } from 'react'
import {
  Button,
  useNotify,
  useRefresh,
  useTranslate,
  useUnselectAll,
} from 'react-admin'
import { useSelector } from 'react-redux'
import SyncIcon from '@mui/icons-material/Sync'
import CachedIcon from '@mui/icons-material/Cached'
import subsonic from '../subsonic'
import type { NavidromeRootState } from '../types/redux'

type LibraryScanButtonProps = {
  fullScan: boolean
  selectedIds?: Array<string | number>
  className?: string
}

const LibraryScanButton = ({
  fullScan,
  selectedIds,
  className,
}: LibraryScanButtonProps) => {
  const [loading, setLoading] = useState(false)
  const notify = useNotify()
  const refresh = useRefresh()
  const translate = useTranslate()
  const unselectAll = useUnselectAll('library')
  const scanStatus = useSelector(
    (state: NavidromeRootState) => state.activity.scanStatus,
  )

  const handleClick = async () => {
    setLoading(true)
    try {
      // Build scan options
      const options: { fullScan: boolean; target?: string[] } = { fullScan }

      // If specific libraries are selected, scan only those
      // Format: "libraryID:" to scan entire library (no folder path specified)
      if (selectedIds && selectedIds.length > 0) {
        options.target = selectedIds.map((id) => `${id}:`)
      }

      await subsonic.startScan(options)
      const notificationKey = fullScan
        ? 'resources.library.notifications.fullScanStarted'
        : 'resources.library.notifications.quickScanStarted'
      notify(notificationKey, { type: 'info' })
      refresh()

      // Unselect all items after successful scan
      unselectAll()
    } catch (error) {
      notify('resources.library.notifications.scanError', { type: 'warning' })
    } finally {
      setLoading(false)
    }
  }

  const isDisabled = loading || scanStatus.scanning

  const label = fullScan
    ? translate('resources.library.actions.fullScan')
    : translate('resources.library.actions.quickScan')

  const icon = fullScan ? <CachedIcon /> : <SyncIcon />

  return (
    <Button
      onClick={handleClick}
      disabled={isDisabled}
      label={label}
      className={className}
    >
      {icon}
    </Button>
  )
}

export default LibraryScanButton
