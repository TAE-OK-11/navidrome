// @ts-nocheck -- legacy JavaScript migration; remove after typing this module
import React, { useMemo } from 'react'
import {
  BulkActionsToolbar,
  FunctionField,
  ListToolbar,
  NumberField,
  TextField,
  useListContext,
} from 'react-admin'
import clsx from 'clsx'
import { useDispatch, useSelector } from 'react-redux'
import { Card, useMediaQuery } from '@mui/material'
import makeStyles from '../themes/makeStyles'
import FavoriteBorderIcon from '@mui/icons-material/FavoriteBorder'
import { playTracks } from '../actions'
import {
  ArtistLinkField,
  DateField,
  DurationField,
  QualityInfo,
  RatingField,
  SizeField,
  SongBulkActions,
  SongContextMenu,
  SongDatagrid,
  SongInfo,
  SongTitleField,
  useResourceRefresh,
  useSelectedFields,
} from '../common'
import config from '../config'
import ExpandInfoDialog from '../dialogs/ExpandInfoDialog'
import { removeAlbumCommentsFromSongs } from './utils'

const useStyles = makeStyles(
  (theme) => ({
    root: {},
    main: {
      display: 'flex',
    },
    content: {
      marginTop: 0,
      transition: theme.transitions.create('margin-top'),
      position: 'relative',
      flex: '1 1 auto',
      [theme.breakpoints.down('sm')]: {
        boxShadow: 'none',
      },
    },
    bulkActionsDisplayed: {
      marginTop: -theme.spacing(8),
      transition: theme.transitions.create('margin-top'),
    },
    actions: {
      zIndex: 2,
      display: 'flex',
      justifyContent: 'flex-end',
      flexWrap: 'wrap',
    },
    noResults: { padding: 20 },
    columnIcon: {
      marginLeft: '3px',
      marginTop: '-2px',
      verticalAlign: 'text-top',
    },
    toolbar: {
      justifyContent: 'flex-start',
    },
    row: {
      '&:hover': {
        '& $contextMenu': {
          visibility: 'visible',
        },
        '& $ratingField': {
          visibility: 'visible',
        },
      },
    },
    contextMenu: {
      visibility: (props) => (props.isDesktop ? 'hidden' : 'visible'),
    },
    ratingField: {
      visibility: 'hidden',
    },
  }),
  { name: 'RaList' },
)

const AlbumSongs = (props) => {
  const records = props.data || []
  const ids = records.map((record) => record.id)
  const dataById = Object.fromEntries(
    records.map((record) => [record.id, record]),
  )
  const isDesktop = useMediaQuery((theme) => theme.breakpoints.up('md'))
  const classes = useStyles({ isDesktop })
  const dispatch = useDispatch()
  const version = useSelector(
    (state) => state.activity?.refresh?.lastReceived || 0,
  )
  useResourceRefresh('song', 'album')

  const toggleableFields = useMemo(() => {
    return {
      trackNumber: isDesktop && (
        <TextField source="trackNumber" label="#" sortable={false} />
      ),
      title: (
        <SongTitleField
          source="title"
          sortable={false}
          showTrackNumbers={!isDesktop}
        />
      ),
      artist: isDesktop && <ArtistLinkField source="artist" sortable={false} />,
      composer: isDesktop && (
        <ArtistLinkField source="composer" sortable={false} />
      ),
      duration: <DurationField source="duration" sortable={false} />,
      year: isDesktop && (
        <FunctionField
          source="year"
          render={(r) => r.year || ''}
          sortable={false}
        />
      ),
      playCount: isDesktop && (
        <NumberField source="playCount" sortable={false} />
      ),
      playDate: <DateField source="playDate" sortable={false} showTime />,
      quality: isDesktop && <QualityInfo source="quality" sortable={false} />,
      size: isDesktop && <SizeField source="size" sortable={false} />,
      channels: isDesktop && <NumberField source="channels" sortable={false} />,
      bpm: isDesktop && <NumberField source="bpm" sortable={false} />,
      genre: <TextField source="genre" sortable={false} />,
      mood: isDesktop && (
        <FunctionField
          source="mood"
          render={(r) => r.tags?.mood?.[0] ?? ''}
          sortable={false}
        />
      ),
      rating: isDesktop && config.enableStarRating && (
        <RatingField
          resource={'song'}
          source="rating"
          sortable={false}
          className={classes.ratingField}
        />
      ),
    }
  }, [isDesktop, classes.ratingField])

  const columns = useSelectedFields({
    resource: 'albumSong',
    columns: toggleableFields,
    omittedColumns: ['title'],
    defaultOff: [
      'composer',
      'channels',
      'bpm',
      'year',
      'playCount',
      'playDate',
      'size',
      'mood',
      'genre',
    ],
  })

  const bulkActionsLabel = isDesktop
    ? 'ra.action.bulk_actions'
    : 'ra.action.bulk_actions_mobile'

  return (
    <>
      <ListToolbar
        classes={{ toolbar: classes.toolbar }}
        actions={props.actions}
        {...props}
      />
      <div className={classes.main}>
        <Card
          className={clsx(classes.content, {
            [classes.bulkActionsDisplayed]: props.selectedIds.length > 0,
          })}
          key={version}
        >
          <BulkActionsToolbar {...props} label={bulkActionsLabel}>
            <SongBulkActions />
          </BulkActionsToolbar>
          <SongDatagrid
            rowClick={(id) => dispatch(playTracks(dataById, ids, id))}
            {...props}
            hasBulkActions={true}
            showDiscSubtitles={true}
            contextAlwaysVisible={!isDesktop}
            classes={{ row: classes.row }}
          >
            {columns}
            <SongContextMenu
              source={'starred'}
              sortable={false}
              className={classes.contextMenu}
              label={
                config.enableFavourites && (
                  <FavoriteBorderIcon
                    fontSize={'small'}
                    className={classes.columnIcon}
                  />
                )
              }
            />
          </SongDatagrid>
        </Card>
      </div>
      <ExpandInfoDialog content={<SongInfo />} />
    </>
  )
}

const SanitizedAlbumSongs = (props) => {
  const context = useListContext()
  removeAlbumCommentsFromSongs({ album: props.album, data: context.data })
  return (
    <>
      {!context.isPending && (
        <AlbumSongs {...context} actions={props.actions} album={props.album} />
      )}
    </>
  )
}

export default SanitizedAlbumSongs
