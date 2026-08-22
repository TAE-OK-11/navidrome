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
import makeStyles from '../themes/makeStyles'
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

const useStyles = makeStyles((theme) => ({
  wrapper: {
    position: 'relative',
    color: (props) =>
      props.serverDown
        ? theme.palette.error.main
        : props.hasWarning
          ? theme.palette.warning.main
          : null,
  },
  progress: {
    color: theme.palette.primary.light,
    position: 'absolute',
    top: 10,
    left: 10,
    zIndex: 1,
  },
  button: {
    color: 'inherit',
    zIndex: 2,
  },
  counterStatus: {
    display: 'grid',
    gridTemplateColumns: 'minmax(0, 1fr) auto',
    alignItems: 'center',
    columnGap: theme.spacing(2),
    width: '100%',
  },
  error: {
    color: theme.palette.error.main,
  },
  card: {
    width: 'clamp(17rem, 78vw, 20rem)',
    maxWidth: 'calc(100vw - 24px)',
  },
  cardContent: {
    padding: theme.spacing(1.5, 2),
    '&:last-child': {
      paddingBottom: theme.spacing(1.5),
    },
  },
  statusLabel: {
    minWidth: 0,
    overflowWrap: 'anywhere',
  },
  statusValue: {
    textAlign: 'right',
    whiteSpace: 'nowrap',
    fontVariantNumeric: 'tabular-nums',
  },
  actions: {
    minHeight: 0,
    padding: theme.spacing(0.5, 1),
    justifyContent: 'flex-end',
  },
}))

const getUptime = (serverStart) =>
  formatDuration((Date.now() - serverStart.startTime) / 1000)

const Uptime = () => {
  const serverStart = useSelector((state) => state.activity.serverStart)
  const [uptime, setUptime] = useState(getUptime(serverStart))
  useInterval(() => {
    setUptime(getUptime(serverStart))
  }, 1000)
  return <span>{uptime}</span>
}

const ActivityPanel = () => {
  const serverStart = useSelector((state) => state.activity.serverStart)
  const up = serverStart.startTime
  const scanStatus = useSelector((state) => state.activity.scanStatus)
  const elapsed = useScanElapsedTime(
    scanStatus.scanning,
    scanStatus.elapsedTime,
  )
  // Determine icon state: error (server down), warning (scan error), or normal
  const serverDown = !up
  const hasWarning = Boolean(scanStatus.error)
  const classes = useStyles({ serverDown, hasWarning })
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
    <div className={classes.wrapper}>
      <Tooltip title={tooltipTitle}>
        <IconButton
          className={classes.button}
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
        <CircularProgress size={24} className={classes.progress} />
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
        <Card className={classes.card}>
          <CardContent className={classes.cardContent}>
            <Box className={classes.counterStatus}>
              <Box component="span" className={classes.statusLabel}>
                {translate('activity.serverUptime')}:
              </Box>
              <Box
                component="span"
                className={`${classes.statusValue} ${!up ? classes.error : ''}`}
              >
                {up ? <Uptime /> : translate('activity.serverDown')}
              </Box>
            </Box>
          </CardContent>
          <Divider />
          <CardContent className={classes.cardContent}>
            <Box className={classes.counterStatus}>
              <Box component="span" className={classes.statusLabel}>
                {translate('activity.totalScanned')}:
              </Box>
              <Box component="span" className={classes.statusValue}>
                {scanStatus.folderCount || '-'}
              </Box>
            </Box>

            <Box
              className={classes.counterStatus}
              sx={{
                mt: 1,
              }}
            >
              <Box component="span" className={classes.statusLabel}>
                {translate('activity.scanType')}:
              </Box>
              <Box component="span" className={classes.statusValue}>
                {lastScanType || '-'}
              </Box>
            </Box>

            <Box
              className={classes.counterStatus}
              sx={{
                mt: 1,
              }}
            >
              <Box component="span" className={classes.statusLabel}>
                {translate('activity.elapsedTime')}:
              </Box>
              <Box component="span" className={classes.statusValue}>
                {formatShortDuration(elapsed)}
              </Box>
            </Box>

            {scanStatus.error && (
              <Box
                className={classes.error}
                sx={{
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
          <CardActions className={classes.actions}>
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
    </div>
  )
}

export default ActivityPanel
