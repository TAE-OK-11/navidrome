import React, { useState, useEffect } from 'react'
import { useSelector } from 'react-redux'
import { useNotify, useTranslate } from 'react-admin'
import {
  Popover,
  CircularProgress,
  IconButton,
  Tooltip,
  Card,
  CardContent,
  CardActions,
  Divider,
  Box,
  Typography,
} from '@mui/material'
import { FiActivity } from 'react-icons/fi'
import { BiError, BiMessageError } from 'react-icons/bi'
import { VscSync } from 'react-icons/vsc'
import { GiMagnifyingGlass } from 'react-icons/gi'
import subsonic from '../subsonic'
import { useInitialScanStatus } from './useInitialScanStatus'
import { useInterval } from '../common'
import { useScanElapsedTime } from './useScanElapsedTime'
import { formatDuration, formatShortDuration } from '../utils'
import config from '../config'
import type { NavidromeRootState } from '../types/redux'

const counterStatusSx = {
  display: 'grid',
  gridTemplateColumns: 'minmax(0, 1fr) auto',
  alignItems: 'center',
  columnGap: 2,
  width: '100%',
}

const cardContentSx = {
  py: 1.5,
  px: 2,
  '&:last-child': { pb: 1.5 },
}

const statusLabelSx = { minWidth: 0, overflowWrap: 'anywhere' }
const statusValueSx = {
  textAlign: 'right',
  whiteSpace: 'nowrap',
  fontVariantNumeric: 'tabular-nums',
}

const getUptime = (serverStart) =>
  formatDuration((Date.now() - serverStart.startTime) / 1000)

const Uptime = () => {
  const serverStart = useSelector(
    (state: NavidromeRootState) => state.activity.serverStart,
  )
  const [uptime, setUptime] = useState(getUptime(serverStart))
  useInterval(() => {
    setUptime(getUptime(serverStart))
  }, 1000)
  return <span>{uptime}</span>
}

const ActivityPanel = () => {
  const serverStart = useSelector(
    (state: NavidromeRootState) => state.activity.serverStart,
  )
  const up = serverStart.startTime
  const scanStatus = useSelector(
    (state: NavidromeRootState) => state.activity.scanStatus,
  )
  const elapsed = useScanElapsedTime(
    scanStatus.scanning,
    scanStatus.elapsedTime,
  )
  // Determine icon state: error (server down), warning (scan error), or normal
  const serverDown = !up
  const hasWarning = Boolean(scanStatus.error)
  const translate = useTranslate()
  const notify = useNotify()
  const [anchorEl, setAnchorEl] = useState(null)
  const open = Boolean(anchorEl)
  useInitialScanStatus()

  const handleMenuOpen = (event) => {
    setAnchorEl(event.currentTarget)
  }

  const handleMenuClose = () => {
    setAnchorEl(null)
  }
  const triggerScan = (full) => () => subsonic.startScan({ fullScan: full })

  useEffect(() => {
    if (serverStart.version && serverStart.version !== config.version) {
      notify('ra.notification.new_version', {
        type: 'info',
        autoHideDuration: 604800000 * 50,
      })
    }
  }, [serverStart, notify])

  const tooltipTitle = scanStatus.error
    ? `${translate('activity.status')}: ${scanStatus.error}`
    : translate('activity.title')

  const lastScanType = (() => {
    switch (scanStatus.scanType) {
      case 'full':
        return translate('activity.fullScan')
      case 'quick':
        return translate('activity.quickScan')
      case 'full-selective':
      case 'quick-selective':
        return translate('activity.selectiveScan')
      default:
        return ''
    }
  })()

  return (
    <Box
      sx={{
        position: 'relative',
        color: serverDown
          ? 'error.main'
          : hasWarning
            ? 'warning.main'
            : undefined,
      }}
    >
      <Tooltip title={tooltipTitle}>
        <IconButton
          sx={{ color: 'inherit', zIndex: 2 }}
          onClick={handleMenuOpen}
          size="large"
        >
          {serverDown ? (
            <BiError data-testid="activity-error-icon" size={'20'} />
          ) : hasWarning ? (
            <BiMessageError data-testid="activity-warning-icon" size={'20'} />
          ) : (
            <FiActivity data-testid="activity-ok-icon" size={'20'} />
          )}
        </IconButton>
      </Tooltip>
      {scanStatus.scanning && (
        <CircularProgress
          size={24}
          sx={{
            color: 'primary.light',
            position: 'absolute',
            top: 10,
            left: 10,
            zIndex: 1,
          }}
        />
      )}
      <Popover
        id="panel-activity"
        anchorEl={anchorEl}
        anchorOrigin={{
          vertical: 'bottom',
          horizontal: 'right',
        }}
        transformOrigin={{
          vertical: 'top',
          horizontal: 'right',
        }}
        open={open}
        onClose={handleMenuClose}
        slotProps={{
          paper: {
            sx: {
              maxWidth: 'calc(100vw - 16px)',
              overflow: 'hidden',
            },
          },
        }}
      >
        <Card
          sx={{
            width: 'clamp(17rem, 78vw, 20rem)',
            maxWidth: 'calc(100vw - 24px)',
          }}
        >
          <CardContent sx={cardContentSx}>
            <Box sx={counterStatusSx}>
              <Box component="span" sx={statusLabelSx}>
                {translate('activity.serverUptime')}:
              </Box>
              <Box
                component="span"
                sx={[statusValueSx, !up && { color: 'error.main' }]}
              >
                {up ? <Uptime /> : translate('activity.serverDown')}
              </Box>
            </Box>
          </CardContent>
          <Divider />
          <CardContent sx={cardContentSx}>
            <Box sx={counterStatusSx}>
              <Box component="span" sx={statusLabelSx}>
                {translate('activity.totalScanned')}:
              </Box>
              <Box component="span" sx={statusValueSx}>
                {scanStatus.folderCount || '-'}
              </Box>
            </Box>

            <Box sx={[counterStatusSx, { mt: 1 }]}>
              <Box component="span" sx={statusLabelSx}>
                {translate('activity.scanType')}:
              </Box>
              <Box component="span" sx={statusValueSx}>
                {lastScanType || '-'}
              </Box>
            </Box>

            <Box sx={[counterStatusSx, { mt: 1 }]}>
              <Box component="span" sx={statusLabelSx}>
                {translate('activity.elapsedTime')}:
              </Box>
              <Box component="span" sx={statusValueSx}>
                {formatShortDuration(elapsed)}
              </Box>
            </Box>

            {scanStatus.error && (
              <Box
                sx={{
                  color: 'error.main',
                  display: 'flex',
                  flexDirection: 'column',
                  mt: 2,
                }}
              >
                <Typography variant="subtitle2">
                  {translate('activity.status')}:
                </Typography>
                <Typography variant="body2">{scanStatus.error}</Typography>
              </Box>
            )}
          </CardContent>
          <Divider />
          <CardActions
            sx={{ minHeight: 0, py: 0.5, px: 1, justifyContent: 'flex-end' }}
          >
            <Tooltip title={translate('activity.quickScan')}>
              <IconButton
                onClick={triggerScan(false)}
                disabled={scanStatus.scanning}
                size="large"
              >
                <VscSync />
              </IconButton>
            </Tooltip>
            <Tooltip title={translate('activity.fullScan')}>
              <IconButton
                onClick={triggerScan(true)}
                disabled={scanStatus.scanning}
                size="large"
              >
                <GiMagnifyingGlass />
              </IconButton>
            </Tooltip>
          </CardActions>
        </Card>
      </Popover>
    </Box>
  )
}

export default ActivityPanel
