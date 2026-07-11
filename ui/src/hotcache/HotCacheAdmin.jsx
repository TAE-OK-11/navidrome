import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { Redirect } from 'react-router-dom'
import { Title, useNotify, usePermissions, useTranslate } from 'react-admin'
import {
  Box,
  Button,
  Chip,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  Divider,
  FormControl,
  IconButton,
  InputLabel,
  LinearProgress,
  makeStyles,
  MenuItem,
  Select,
  Tab,
  TablePagination,
  Tabs,
  TextField,
  Tooltip,
  Typography,
} from '@material-ui/core'
import RefreshIcon from '@material-ui/icons/Refresh'
import PauseIcon from '@material-ui/icons/Pause'
import PlayArrowIcon from '@material-ui/icons/PlayArrow'
import VerifiedUserIcon from '@material-ui/icons/VerifiedUser'
import DeleteSweepIcon from '@material-ui/icons/DeleteSweep'
import BuildIcon from '@material-ui/icons/Build'
import CloudDownloadIcon from '@material-ui/icons/CloudDownload'
import {
  getHotCacheCandidates,
  getHotCacheDashboard,
  getHotCacheEntries,
  hotCacheAction,
  promoteHotCacheCandidates,
} from './api'
import {
  formatDurationNs,
  formatNumber,
  formatRate,
  formatStorage,
} from './formatters'
import {
  CurrentPromotion,
  CandidatesTable,
  EntriesTable,
  EventsTable,
  FormatsTable,
  QueueTable,
  SessionsTable,
} from './HotCacheTables'

const useStyles = makeStyles((theme) => ({
  root: { marginTop: theme.spacing(1), minWidth: 0 },
  header: {
    display: 'flex',
    alignItems: 'center',
    gap: theme.spacing(1),
    flexWrap: 'wrap',
    marginBottom: theme.spacing(2),
  },
  headerTitle: { flex: 1, minWidth: 220 },
  metrics: {
    display: 'grid',
    gridTemplateColumns: 'repeat(auto-fit, minmax(150px, 1fr))',
    borderTop: `1px solid ${theme.palette.divider}`,
    borderLeft: `1px solid ${theme.palette.divider}`,
  },
  metric: {
    minHeight: 72,
    padding: theme.spacing(1.25),
    borderRight: `1px solid ${theme.palette.divider}`,
    borderBottom: `1px solid ${theme.palette.divider}`,
  },
  metricValue: {
    fontSize: '1.05rem',
    fontWeight: 600,
    overflowWrap: 'anywhere',
  },
  tabs: {
    marginTop: theme.spacing(2),
    borderBottom: `1px solid ${theme.palette.divider}`,
  },
  panel: { paddingTop: theme.spacing(2), minWidth: 0 },
  toolbar: {
    display: 'flex',
    alignItems: 'center',
    gap: theme.spacing(1),
    flexWrap: 'wrap',
    marginBottom: theme.spacing(1),
  },
  search: { minWidth: 220 },
  select: { minWidth: 135 },
  spacer: { flex: 1 },
  progress: { height: 4 },
}))

const Metric = ({ label, value }) => {
  const classes = useStyles()
  return (
    <Box className={classes.metric}>
      <Typography variant="caption" color="textSecondary">
        {label}
      </Typography>
      <Typography className={classes.metricValue}>{value}</Typography>
    </Box>
  )
}

const errorStatus = (error) =>
  Number(error?.status || error?.response?.status || 0)

const HotCacheAdmin = () => {
  const classes = useStyles()
  const translate = useTranslate()
  const notify = useNotify()
  const { permissions, loading } = usePermissions()
  const [tab, setTab] = useState(0)
  const [dashboard, setDashboard] = useState({
    status: null,
    sessions: [],
    queue: [],
    current: null,
    formats: [],
    events: [],
    errors: [],
    artwork: [],
  })
  const [entries, setEntries] = useState({ items: [], total: 0 })
  const [entryQuery, setEntryQuery] = useState({
    search: '',
    format: '',
    state: '',
    sort: 'recent',
    order: 'desc',
    offset: 0,
    limit: 25,
  })
  const [debouncedEntryQuery, setDebouncedEntryQuery] = useState(entryQuery)
  const [candidateSearch, setCandidateSearch] = useState('')
  const [candidateQuery, setCandidateQuery] = useState('')
  const [candidates, setCandidates] = useState({ items: [], hasMore: false })
  const [selectedCandidates, setSelectedCandidates] = useState([])
  const [busy, setBusy] = useState(false)
  const [initialLoading, setInitialLoading] = useState(true)
  const [confirm, setConfirm] = useState(null)
  const pollErrorShown = useRef(false)

  const t = useCallback(
    (key, options) => translate(`hotCache.${key}`, options),
    [translate],
  )
  const tableLabels = useMemo(
    () => new Proxy({}, { get: (_, key) => t(`columns.${String(key)}`) }),
    [t],
  )

  const loadDashboard = useCallback(async (signal) => {
    const value = await getHotCacheDashboard(signal)
    setDashboard(value)
    pollErrorShown.current = false
    setInitialLoading(false)
  }, [])

  const loadEntries = useCallback(
    async (signal) => {
      setEntries(await getHotCacheEntries(debouncedEntryQuery, signal))
    },
    [debouncedEntryQuery],
  )

  const loadCandidates = useCallback(
    async (signal) => {
      if (candidateQuery.trim().length < 2) {
        setCandidates({ items: [], hasMore: false })
        setSelectedCandidates([])
        return
      }
      const page = await getHotCacheCandidates(
        { search: candidateQuery.trim(), limit: 25 },
        signal,
      )
      setCandidates(page)
      const available = new Set(
        page.items
          .filter((item) => item.cacheState === 'available')
          .map((item) => item.mediaId),
      )
      setSelectedCandidates((current) =>
        current.filter((mediaId) => available.has(mediaId)),
      )
    },
    [candidateQuery],
  )

  const refresh = useCallback(async () => {
    const controller = new AbortController()
    setBusy(true)
    try {
      const tasks = [loadDashboard(controller.signal)]
      if (tab === 3) tasks.push(loadEntries(controller.signal))
      if (tab === 4) tasks.push(loadCandidates(controller.signal))
      await Promise.all(tasks)
    } catch (error) {
      if (error.name !== 'AbortError') notify(error.message, 'warning')
    } finally {
      setBusy(false)
    }
  }, [loadCandidates, loadDashboard, loadEntries, notify, tab])

  useEffect(() => {
    const timer = window.setTimeout(
      () => setDebouncedEntryQuery(entryQuery),
      350,
    )
    return () => window.clearTimeout(timer)
  }, [entryQuery])

  useEffect(() => {
    const timer = window.setTimeout(
      () => setCandidateQuery(candidateSearch),
      350,
    )
    return () => window.clearTimeout(timer)
  }, [candidateSearch])

  useEffect(() => {
    if (permissions !== 'admin') return undefined
    let active = true
    let controller
    let timer
    const schedule = (delay) => {
      window.clearTimeout(timer)
      if (active) timer = window.setTimeout(poll, delay)
    }
    const poll = async () => {
      if (document.hidden) {
        schedule(15000)
        return
      }
      controller = new AbortController()
      let delay = 15000
      try {
        await loadDashboard(controller.signal)
      } catch (error) {
        delay = errorStatus(error) === 429 ? 60000 : 15000
        if (active && error.name !== 'AbortError' && !pollErrorShown.current) {
          notify(error.message, 'warning')
          pollErrorShown.current = true
        }
        setInitialLoading(false)
      }
      schedule(delay)
    }
    poll()
    const onVisibilityChange = () => {
      if (!document.hidden) schedule(250)
    }
    document.addEventListener('visibilitychange', onVisibilityChange)
    return () => {
      active = false
      controller?.abort()
      window.clearTimeout(timer)
      document.removeEventListener('visibilitychange', onVisibilityChange)
    }
  }, [loadDashboard, notify, permissions])

  useEffect(() => {
    if (permissions !== 'admin' || tab !== 3) return undefined
    const controller = new AbortController()
    loadEntries(controller.signal).catch(
      (error) =>
        error.name !== 'AbortError' && notify(error.message, 'warning'),
    )
    return () => controller.abort()
  }, [loadEntries, notify, permissions, tab])

  useEffect(() => {
    if (permissions !== 'admin' || tab !== 4) return undefined
    const controller = new AbortController()
    loadCandidates(controller.signal).catch(
      (error) =>
        error.name !== 'AbortError' && notify(error.message, 'warning'),
    )
    return () => controller.abort()
  }, [loadCandidates, notify, permissions, tab])

  const action = useCallback(
    async (path, method = 'POST', headers) => {
      setBusy(true)
      try {
        await hotCacheAction(path, method, headers)
        notify('hotCache.actionAccepted', 'info')
        const tasks = [loadDashboard()]
        if (tab === 3) tasks.push(loadEntries())
        if (tab === 4) tasks.push(loadCandidates())
        await Promise.all(tasks)
      } catch (error) {
        notify(error.message, 'warning')
      } finally {
        setBusy(false)
      }
    },
    [loadCandidates, loadDashboard, loadEntries, notify, tab],
  )

  const toggleCandidate = useCallback((mediaId) => {
    setSelectedCandidates((current) =>
      current.includes(mediaId)
        ? current.filter((id) => id !== mediaId)
        : [...current, mediaId],
    )
  }, [])

  const promoteSelected = useCallback(async () => {
    if (!selectedCandidates.length) return
    setBusy(true)
    try {
      const result = await promoteHotCacheCandidates(selectedCandidates)
      notify(
        t('manual.accepted', { count: result.accepted.length }),
        result.accepted.length ? 'info' : 'warning',
      )
      setSelectedCandidates([])
      await Promise.all([loadDashboard(), loadCandidates()])
    } catch (error) {
      notify(error.message, 'warning')
    } finally {
      setBusy(false)
    }
  }, [loadCandidates, loadDashboard, notify, selectedCandidates, t])

  if (loading) return <LinearProgress />
  if (permissions !== 'admin') return <Redirect to="/" />
  const status = dashboard.status || {}
  const metrics = [
    [
      'used',
      `${formatStorage(status.usedBytes)} / ${formatStorage(status.maxBytes)}`,
    ],
    ['reserved', formatStorage(status.reservedBytes)],
    ['pinned', formatStorage(status.pinnedBytes)],
    ['entries', formatNumber(status.entries)],
    ['queue', formatNumber(status.queueLength)],
    ['sessions', formatNumber(status.activeSessions)],
    ['requestHitRate', formatRate(status.requestHitRate)],
    ['sessionHitRate', formatRate(status.sessionHitRate)],
    ['hitTtfb', formatDurationNs(status.averageHitTtfb)],
    ['missTtfb', formatDurationNs(status.averageMissTtfb)],
    ['fallbacks', formatNumber(status.fallbacks)],
    ['evictions', formatNumber(status.evictions)],
    ['expectedCancellations', formatNumber(status.expectedCancellations)],
    ['transportErrors', formatNumber(status.unexpectedTransportErrors)],
    ['errors24h', formatNumber(status.errors24h)],
    ['artworkErrors24h', formatNumber(status.artworkErrors24h)],
  ]

  return (
    <Box className={classes.root}>
      <Title title={`Navidrome - ${t('title')}`} />
      <Box className={classes.progress}>
        {(busy || initialLoading) && <LinearProgress />}
      </Box>
      <Box className={classes.header}>
        <Box className={classes.headerTitle}>
          <Typography variant="h5">{t('title')}</Typography>
        </Box>
        <Chip
          label={status.health || 'unknown'}
          color={status.health === 'healthy' ? 'primary' : 'default'}
        />
        <Tooltip title={t('refresh')}>
          <IconButton onClick={refresh}>
            <RefreshIcon />
          </IconButton>
        </Tooltip>
        <Button
          size="small"
          startIcon={status.paused ? <PlayArrowIcon /> : <PauseIcon />}
          onClick={() => action(status.paused ? 'resume' : 'pause')}
        >
          {status.paused ? t('resume') : t('pause')}
        </Button>
        <Button
          size="small"
          startIcon={<VerifiedUserIcon />}
          onClick={() => action('verify')}
        >
          {t('verify')}
        </Button>
        <Button
          size="small"
          startIcon={<BuildIcon />}
          onClick={() => action('cleanup')}
        >
          {t('cleanup')}
        </Button>
        <Button
          size="small"
          color="secondary"
          startIcon={<DeleteSweepIcon />}
          onClick={() => setConfirm({ type: 'purge', title: t('purge') })}
        >
          {t('purge')}
        </Button>
      </Box>
      <Box className={classes.metrics}>
        {metrics.map(([label, value]) => (
          <Metric key={label} label={t(`metrics.${label}`)} value={value} />
        ))}
      </Box>
      <Tabs
        className={classes.tabs}
        value={tab}
        onChange={(_, value) => setTab(value)}
        variant="scrollable"
        scrollButtons="auto"
      >
        {[
          'overview',
          'sessions',
          'queue',
          'entries',
          'manual',
          'formats',
          'events',
          'errors',
          'artwork',
        ].map((key) => (
          <Tab key={key} label={t(`tabs.${key}`)} />
        ))}
      </Tabs>
      <Box className={classes.panel}>
        {tab === 0 && (
          <>
            <Typography variant="h6">{t('currentPromotion')}</Typography>
            <CurrentPromotion
              value={dashboard.current}
              labels={{ empty: t('noActivePromotion') }}
            />
            <Divider />
          </>
        )}
        {tab === 1 && (
          <SessionsTable rows={dashboard.sessions} labels={tableLabels} />
        )}
        {tab === 2 && (
          <QueueTable
            rows={dashboard.queue}
            labels={tableLabels}
            onCancel={(id) => action(`queue/${id}/cancel`)}
          />
        )}
        {tab === 3 && (
          <>
            <Box className={classes.toolbar}>
              <TextField
                className={classes.search}
                size="small"
                variant="outlined"
                label={t('search')}
                value={entryQuery.search}
                onChange={(event) =>
                  setEntryQuery({
                    ...entryQuery,
                    search: event.target.value,
                    offset: 0,
                  })
                }
              />
              <FormControl
                className={classes.select}
                size="small"
                variant="outlined"
              >
                <InputLabel>{t('format')}</InputLabel>
                <Select
                  label={t('format')}
                  value={entryQuery.format}
                  onChange={(event) =>
                    setEntryQuery({
                      ...entryQuery,
                      format: event.target.value,
                      offset: 0,
                    })
                  }
                >
                  <MenuItem value="">{t('all')}</MenuItem>
                  {['alac', 'aac', 'flac', 'mp3', 'opus', 'vorbis', 'wav'].map(
                    (value) => (
                      <MenuItem key={value} value={value}>
                        {value.toUpperCase()}
                      </MenuItem>
                    ),
                  )}
                </Select>
              </FormControl>
              <FormControl
                className={classes.select}
                size="small"
                variant="outlined"
              >
                <InputLabel>{t('state')}</InputLabel>
                <Select
                  label={t('state')}
                  value={entryQuery.state}
                  onChange={(event) =>
                    setEntryQuery({
                      ...entryQuery,
                      state: event.target.value,
                      offset: 0,
                    })
                  }
                >
                  <MenuItem value="">{t('all')}</MenuItem>
                  {['pinned', 'playing', 'corrupted', 'recently-used'].map(
                    (value) => (
                      <MenuItem key={value} value={value}>
                        {t(`states.${value}`)}
                      </MenuItem>
                    ),
                  )}
                </Select>
              </FormControl>
            </Box>
            <EntriesTable
              rows={entries.items || []}
              labels={tableLabels}
              onPromote={(id) => action(`entries/${id}/promote`)}
              onRemove={(id, title) =>
                setConfirm({ type: 'remove', id, title })
              }
            />
            <TablePagination
              component="div"
              count={entries.total || 0}
              page={Math.floor(entryQuery.offset / entryQuery.limit)}
              rowsPerPage={entryQuery.limit}
              onChangePage={(_, page) =>
                setEntryQuery({
                  ...entryQuery,
                  offset: page * entryQuery.limit,
                })
              }
              onChangeRowsPerPage={(event) =>
                setEntryQuery({
                  ...entryQuery,
                  limit: Number(event.target.value),
                  offset: 0,
                })
              }
              rowsPerPageOptions={[10, 25, 50, 100]}
            />
          </>
        )}
        {tab === 4 && (
          <>
            <Box className={classes.toolbar}>
              <TextField
                className={classes.search}
                size="small"
                variant="outlined"
                label={t('manual.search')}
                value={candidateSearch}
                onChange={(event) => setCandidateSearch(event.target.value)}
              />
              <Typography variant="caption" color="textSecondary">
                {candidateSearch.trim().length < 2
                  ? t('manual.minimumSearch')
                  : candidates.hasMore
                    ? t('manual.moreResults')
                    : t('manual.resultCount', {
                        count: candidates.items.length,
                      })}
              </Typography>
              <Box className={classes.spacer} />
              <Button
                color="primary"
                variant="contained"
                startIcon={<CloudDownloadIcon />}
                disabled={!selectedCandidates.length || busy}
                onClick={promoteSelected}
              >
                {t('manual.cacheSelected', {
                  count: selectedCandidates.length,
                })}
              </Button>
            </Box>
            <CandidatesTable
              rows={candidates.items}
              labels={tableLabels}
              selected={selectedCandidates}
              onToggle={toggleCandidate}
            />
          </>
        )}
        {tab === 5 && (
          <FormatsTable rows={dashboard.formats} labels={tableLabels} />
        )}
        {tab === 6 && (
          <>
            <Box className={classes.toolbar}>
              <Box className={classes.spacer} />
              <Button size="small" onClick={() => action('events', 'DELETE')}>
                {t('reset')}
              </Button>
            </Box>
            <EventsTable rows={dashboard.events} labels={tableLabels} />
          </>
        )}
        {tab === 7 && (
          <EventsTable rows={dashboard.errors} labels={tableLabels} />
        )}
        {tab === 8 && (
          <>
            <Box className={classes.toolbar}>
              <Box className={classes.spacer} />
              <Button size="small" onClick={() => action('artwork/recheck')}>
                {t('recheck')}
              </Button>
            </Box>
            <EventsTable rows={dashboard.artwork} labels={tableLabels} />
          </>
        )}
      </Box>
      <Dialog open={Boolean(confirm)} onClose={() => setConfirm(null)}>
        <DialogTitle>
          {confirm?.type === 'purge' ? t('purge') : t('remove')}
        </DialogTitle>
        <DialogContent>
          <Typography>
            {confirm?.type === 'purge'
              ? t('confirmPurge')
              : t('confirmRemove', { title: confirm?.title || confirm?.id })}
          </Typography>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setConfirm(null)}>{t('cancel')}</Button>
          <Button
            color="secondary"
            onClick={() => {
              const current = confirm
              setConfirm(null)
              current.type === 'purge'
                ? action('purge', 'POST', { 'X-ND-Confirm': 'purge-hot-cache' })
                : action(`entries/${current.id}`, 'DELETE')
            }}
          >
            {t('confirm')}
          </Button>
        </DialogActions>
      </Dialog>
    </Box>
  )
}

export default HotCacheAdmin
