import React from 'react'
import PropTypes from 'prop-types'
import {
  Box,
  Checkbox,
  Chip,
  IconButton,
  LinearProgress,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  Tooltip,
  Typography,
} from '@material-ui/core'
import DeleteIcon from '@material-ui/icons/Delete'
import CancelIcon from '@material-ui/icons/Cancel'
import CloudDownloadIcon from '@material-ui/icons/CloudDownload'
import {
  formatDate,
  formatDurationNs,
  formatMicros,
  formatNumber,
  formatRate,
  formatStorage,
} from './formatters'

const EmptyRow = ({ columns, label }) => (
  <TableRow>
    <TableCell colSpan={columns} align="center">
      <Box py={3} color="text.secondary">
        {label}
      </Box>
    </TableCell>
  </TableRow>
)

EmptyRow.propTypes = { columns: PropTypes.number, label: PropTypes.string }

export const CandidatesTable = ({ rows, labels, selected, onToggle }) => (
  <TableContainer>
    <Table size="small">
      <TableHead>
        <TableRow>
          <TableCell padding="checkbox" />
          {['track', 'format', 'size', 'state'].map((key) => (
            <TableCell key={key}>{labels[key]}</TableCell>
          ))}
        </TableRow>
      </TableHead>
      <TableBody>
        {!rows.length && <EmptyRow columns={5} label={labels.empty} />}
        {rows.map((row) => {
          const available = row.cacheState === 'available'
          return (
            <TableRow
              key={row.mediaId}
              hover
              selected={selected.includes(row.mediaId)}
            >
              <TableCell padding="checkbox">
                <Checkbox
                  color="primary"
                  checked={selected.includes(row.mediaId)}
                  disabled={!available}
                  inputProps={{ 'aria-label': row.title || row.mediaId }}
                  onChange={() => onToggle(row.mediaId)}
                />
              </TableCell>
              <TableCell>
                <Typography variant="body2">
                  {row.title || row.mediaId}
                </Typography>
                <Typography variant="caption" color="textSecondary">
                  {row.artist || '-'} {row.album ? `- ${row.album}` : ''}
                </Typography>
              </TableCell>
              <TableCell>
                {row.codec || '-'} / {row.container || '-'}
              </TableCell>
              <TableCell>{formatStorage(row.size)}</TableCell>
              <TableCell>
                <Chip
                  size="small"
                  color={row.cacheState === 'cached' ? 'primary' : 'default'}
                  label={labels[`cacheState_${row.cacheState || 'available'}`]}
                />
              </TableCell>
            </TableRow>
          )
        })}
      </TableBody>
    </Table>
  </TableContainer>
)

CandidatesTable.propTypes = {
  rows: PropTypes.array,
  labels: PropTypes.object,
  selected: PropTypes.array,
  onToggle: PropTypes.func,
}

export const SessionsTable = ({ rows, labels }) => (
  <TableContainer>
    <Table size="small">
      <TableHead>
        <TableRow>
          {[
            'track',
            'userPlayer',
            'format',
            'path',
            'progress',
            'requests',
            'state',
            'lastActivity',
          ].map((key) => (
            <TableCell key={key}>{labels[key]}</TableCell>
          ))}
        </TableRow>
      </TableHead>
      <TableBody>
        {!rows.length && <EmptyRow columns={8} label={labels.empty} />}
        {rows.map((row) => (
          <TableRow key={row.id} hover>
            <TableCell>
              <Typography variant="body2">
                {row.title || row.mediaId}
              </Typography>
              <Typography variant="caption" color="textSecondary">
                {row.artist} {row.album ? `- ${row.album}` : ''}
              </Typography>
            </TableCell>
            <TableCell>
              {row.user || '-'} / {row.player || '-'}
            </TableCell>
            <TableCell>
              {row.codec || '-'} / {row.container || '-'}
            </TableCell>
            <TableCell>
              <Chip
                size="small"
                label={row.transferPath}
                color={row.cached ? 'primary' : 'default'}
              />
            </TableCell>
            <TableCell>
              {formatDurationNs(row.playedDuration)} /{' '}
              {formatRate((row.playedPercent || 0) / 100)}
            </TableCell>
            <TableCell>
              {formatNumber(row.requestCount)} / {formatNumber(row.rangeCount)}{' '}
              / {formatNumber(row.seekCount)}
            </TableCell>
            <TableCell>
              {labels[`thresholdState_${row.thresholdState}`]}
            </TableCell>
            <TableCell>{formatDate(row.lastActivity)}</TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  </TableContainer>
)

SessionsTable.propTypes = { rows: PropTypes.array, labels: PropTypes.object }

export const QueueTable = ({ rows, labels, onCancel }) => (
  <TableContainer>
    <Table size="small">
      <TableHead>
        <TableRow>
          {[
            'position',
            'track',
            'format',
            'size',
            'progress',
            'reason',
            'queued',
            'state',
            'actions',
          ].map((key) => (
            <TableCell key={key}>{labels[key]}</TableCell>
          ))}
        </TableRow>
      </TableHead>
      <TableBody>
        {!rows.length && <EmptyRow columns={9} label={labels.empty} />}
        {rows.map((row) => (
          <TableRow key={`${row.mediaId}-${row.queuedAt}`} hover>
            <TableCell>{row.position}</TableCell>
            <TableCell>{row.title || row.mediaId}</TableCell>
            <TableCell>
              {row.codec || '-'} / {row.container || '-'}
            </TableCell>
            <TableCell>{formatStorage(row.sourceSize)}</TableCell>
            <TableCell>
              {formatDurationNs(row.playedDuration)} /{' '}
              {Number(row.playedPercent || 0).toFixed(1)}%
            </TableCell>
            <TableCell>{row.thresholdReason}</TableCell>
            <TableCell>{formatDate(row.queuedAt)}</TableCell>
            <TableCell>{labels[`queueState_${row.state}`]}</TableCell>
            <TableCell>
              <Tooltip title={labels.cancel}>
                <IconButton size="small" onClick={() => onCancel(row.mediaId)}>
                  <CancelIcon fontSize="small" />
                </IconButton>
              </Tooltip>
            </TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  </TableContainer>
)

QueueTable.propTypes = {
  rows: PropTypes.array,
  labels: PropTypes.object,
  onCancel: PropTypes.func,
}

export const EntriesTable = ({ rows, labels, onPromote, onRemove }) => (
  <TableContainer>
    <Table size="small">
      <TableHead>
        <TableRow>
          {[
            'track',
            'format',
            'size',
            'lastHit',
            'hits',
            'integrity',
            'lru',
            'ttfb',
            'actions',
          ].map((key) => (
            <TableCell key={key}>{labels[key]}</TableCell>
          ))}
        </TableRow>
      </TableHead>
      <TableBody>
        {!rows.length && <EmptyRow columns={9} label={labels.empty} />}
        {rows.map((row) => (
          <TableRow key={row.mediaId} hover>
            <TableCell>
              <Typography variant="body2">
                {row.title || row.mediaId}
              </Typography>
              <Typography variant="caption" color="textSecondary">
                {row.artist} {row.album ? `- ${row.album}` : ''}
              </Typography>
            </TableCell>
            <TableCell>
              {row.codec || '-'} / {row.container || row.extension || '-'}
            </TableCell>
            <TableCell>{formatStorage(row.fileSize)}</TableCell>
            <TableCell>{formatDate(row.lastRequestHit)}</TableCell>
            <TableCell>
              {formatNumber(row.sessionHits)} / {formatNumber(row.requestHits)}
            </TableCell>
            <TableCell>
              <Chip
                size="small"
                label={labels[`integrity_${row.integrityState}`]}
                color={
                  row.integrityState === 'verified' ? 'primary' : 'secondary'
                }
              />
            </TableCell>
            <TableCell>#{row.lruRank}</TableCell>
            <TableCell>{formatDurationNs(row.latestTtfb)}</TableCell>
            <TableCell>
              <Tooltip title={labels.promote}>
                <span>
                  <IconButton
                    size="small"
                    disabled={row.pinned || row.playing}
                    onClick={() => onPromote(row.mediaId)}
                  >
                    <CloudDownloadIcon fontSize="small" />
                  </IconButton>
                </span>
              </Tooltip>
              <Tooltip title={labels.remove}>
                <span>
                  <IconButton
                    size="small"
                    disabled={row.pinned || row.playing}
                    onClick={() => onRemove(row.mediaId, row.title)}
                  >
                    <DeleteIcon fontSize="small" />
                  </IconButton>
                </span>
              </Tooltip>
            </TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  </TableContainer>
)

EntriesTable.propTypes = {
  rows: PropTypes.array,
  labels: PropTypes.object,
  onPromote: PropTypes.func,
  onRemove: PropTypes.func,
}

export const FormatsTable = ({ rows, labels }) => (
  <TableContainer>
    <Table size="small">
      <TableHead>
        <TableRow>
          {[
            'format',
            'entries',
            'bytes',
            'requestHitRate',
            'sessionHitRate',
            'ttfb',
            'throughput',
            'sendfile',
            'ranges',
            'fallbacks',
          ].map((key) => (
            <TableCell key={key}>{labels[key]}</TableCell>
          ))}
        </TableRow>
      </TableHead>
      <TableBody>
        {rows.map((row) => (
          <TableRow key={row.format} hover>
            <TableCell>{row.format}</TableCell>
            <TableCell>{formatNumber(row.entries)}</TableCell>
            <TableCell>{formatStorage(row.bytes)}</TableCell>
            <TableCell>{formatRate(row.requestHitRate)}</TableCell>
            <TableCell>{formatRate(row.sessionHitRate)}</TableCell>
            <TableCell>
              {formatMicros(row.ttfbP50Micros)} /{' '}
              {formatMicros(row.ttfbP95Micros)} /{' '}
              {formatMicros(row.ttfbP99Micros)}
            </TableCell>
            <TableCell>
              {formatStorage(row.throughputBytesPerSecond)}/s
            </TableCell>
            <TableCell>{formatRate(row.sendfileRate)}</TableCell>
            <TableCell>{formatNumber(row.rangeRequests)}</TableCell>
            <TableCell>{formatNumber(row.fallbacks)}</TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  </TableContainer>
)

FormatsTable.propTypes = { rows: PropTypes.array, labels: PropTypes.object }

export const EventsTable = ({ rows, labels }) => (
  <TableContainer>
    <Table size="small">
      <TableHead>
        <TableRow>
          {['time', 'type', 'category', 'track', 'reason', 'message'].map(
            (key) => (
              <TableCell key={key}>{labels[key]}</TableCell>
            ),
          )}
        </TableRow>
      </TableHead>
      <TableBody>
        {!rows.length && <EmptyRow columns={6} label={labels.empty} />}
        {rows.map((row) => (
          <TableRow key={row.id} hover>
            <TableCell>{formatDate(row.timestamp)}</TableCell>
            <TableCell>{row.type}</TableCell>
            <TableCell>{row.category}</TableCell>
            <TableCell>{row.title || row.mediaId || '-'}</TableCell>
            <TableCell>{row.reason || row.code || '-'}</TableCell>
            <TableCell>{row.message || '-'}</TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  </TableContainer>
)

EventsTable.propTypes = { rows: PropTypes.array, labels: PropTypes.object }

export const CurrentPromotion = ({ value, labels }) => {
  if (!value)
    return (
      <Box py={3} color="text.secondary">
        {labels.empty}
      </Box>
    )
  return (
    <Box py={2}>
      <Box display="flex" justifyContent="space-between" mb={1}>
        <Typography variant="body2">{value.title || value.mediaId}</Typography>
        <Typography variant="body2">
          {Number(value.progress || 0).toFixed(1)}%
        </Typography>
      </Box>
      <LinearProgress
        variant="determinate"
        value={Math.min(100, value.progress || 0)}
      />
      <Box
        display="flex"
        flexWrap="wrap"
        gridGap={16}
        mt={1}
        color="text.secondary"
      >
        <span>{labels.phase(value.phase)}</span>
        <span>
          {formatStorage(value.bytesCopied)} / {formatStorage(value.totalBytes)}
        </span>
        <span>{formatStorage(value.speed)}/s</span>
        <span>{formatDurationNs(value.elapsed)}</span>
      </Box>
    </Box>
  )
}

CurrentPromotion.propTypes = {
  value: PropTypes.object,
  labels: PropTypes.object,
}
