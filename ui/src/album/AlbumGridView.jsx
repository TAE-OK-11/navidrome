import React from 'react'
import { Typography, ImageListItemBar, useMediaQuery } from '@mui/material'
import makeStyles from '../themes/makeStyles'
import { Link } from 'react-router-dom'
import { useListContext, Loading } from 'react-admin'
import { linkToRecord } from '../utils/linkToRecord'
import { useDrag } from 'react-dnd'
import subsonic from '../subsonic'
import {
  AlbumContextMenu,
  PlayButton,
  ArtistLinkField,
  OverflowTooltip,
  useImageUrl,
} from '../common'
import config from '../config'
import { DraggableTypes } from '../consts'
import clsx from 'clsx'
import { AlbumDatesField } from './AlbumDatesField.jsx'

// FIXME checkout https://mui.com/components/use-media-query/#migrating-from-withwidth
const withWidth = () => (WrappedComponent) => {
  const WithWidth = (props) => <WrappedComponent {...props} width="xs" />
  WithWidth.displayName = `WithWidth(${WrappedComponent.displayName || WrappedComponent.name || 'Component'})`
  return WithWidth
}

const useStyles = makeStyles(
  (theme) => ({
    root: {
      margin: '20px',
      minWidth: 0,
    },
    grid: {
      display: 'grid',
      gap: '20px',
      minWidth: 0,
      width: '100%',
    },
    gridListTile: {
      minWidth: 0,
      width: '100%',
    },
    tileBar: {
      transition: 'all 150ms ease-out',
      opacity: 0,
      pointerEvents: 'none',
      textAlign: 'left',
      background:
        'linear-gradient(to top, rgba(0,0,0,0.7) 0%,rgba(0,0,0,0.4) 70%,rgba(0,0,0,0) 100%)',
    },
    tileBarMobile: {
      textAlign: 'left',
      background:
        'linear-gradient(to top, rgba(0,0,0,0.7) 0%,rgba(0,0,0,0.4) 70%,rgba(0,0,0,0) 100%)',
    },
    albumArtistName: {
      whiteSpace: 'nowrap',
      overflow: 'hidden',
      textOverflow: 'ellipsis',
      textAlign: 'left',
      fontSize: '1em',
    },
    albumName: {
      fontSize: '14px',
      color: theme.palette.mode === 'dark' ? '#eee' : 'black',
      overflow: 'hidden',
      whiteSpace: 'nowrap',
      textOverflow: 'ellipsis',
    },
    missingAlbum: {
      opacity: 0.3,
    },
    albumVersion: {
      fontSize: '12px',
      color: theme.palette.mode === 'dark' ? '#c5c5c5' : '#696969',
      overflow: 'hidden',
      whiteSpace: 'nowrap',
      textOverflow: 'ellipsis',
    },
    albumSubtitle: {
      fontSize: '12px',
      color: theme.palette.mode === 'dark' ? '#c5c5c5' : '#696969',
      overflow: 'hidden',
      whiteSpace: 'nowrap',
      textOverflow: 'ellipsis',
    },
    link: {
      position: 'relative',
      display: 'block',
      textDecoration: 'none',
      '&:hover $tileBar, &:focus-within $tileBar': {
        opacity: 1,
        pointerEvents: 'auto',
      },
    },
    albumLink: {
      position: 'relative',
      display: 'block',
      textDecoration: 'none',
    },
    albumContainer: {
      minWidth: 0,
      width: '100%',
    },
    albumPlayButton: { color: 'white' },
  }),
  { name: 'NDAlbumGridView' },
)

const useCoverStyles = makeStyles({
  coverContainer: {
    width: '100%',
    aspectRatio: '1 / 1',
    overflow: 'hidden',
  },
  coverContent: {
    width: '100%',
    height: '100%',
  },
  cover: {
    display: 'block',
    width: '100%',
    height: '100%',
    objectFit: 'contain',
    transition: 'opacity 0.3s ease-in-out',
  },
  coverLoading: {
    opacity: 0,
  },
})

const getColsForWidth = (width) => {
  if (width === 'xs') return 2
  if (width === 'sm') return 3
  if (width === 'md') return 4
  if (width === 'lg') return 6
  return 9
}

const Cover = ({ record }) => {
  const classes = useCoverStyles()
  const [, dragAlbumRef] = useDrag(
    () => ({
      type: DraggableTypes.ALBUM,
      item: { albumIds: [record.id] },
      options: { dropEffect: 'copy' },
    }),
    [record],
  )

  const url = subsonic.getCoverArtUrl(record, config.uiCoverArtSize, true)
  const { imgUrl, loading: imageLoading } = useImageUrl(url)

  return (
    <div className={classes.coverContainer}>
      <div ref={dragAlbumRef} className={classes.coverContent}>
        <img
          src={imgUrl || undefined}
          alt={record.name}
          className={`${classes.cover} ${imageLoading ? classes.coverLoading : ''}`}
        />
      </div>
    </div>
  )
}

const AlbumGridTile = ({ showArtist, record, basePath, ...props }) => {
  const classes = useStyles()
  const isDesktop = useMediaQuery((theme) => theme.breakpoints.up('md'), {
    noSsr: true,
  })
  if (!record) {
    return null
  }
  const computedClasses = clsx(
    classes.albumContainer,
    record.missing && classes.missingAlbum,
  )
  return (
    <div className={computedClasses}>
      <Link
        className={classes.link}
        to={linkToRecord(basePath, record.id, 'show')}
      >
        <Cover record={record} />
        <ImageListItemBar
          className={isDesktop ? classes.tileBar : classes.tileBarMobile}
          subtitle={
            !record.missing && (
              <PlayButton
                className={classes.albumPlayButton}
                record={record}
                size="small"
              />
            )
          }
          actionIcon={<AlbumContextMenu record={record} color={'white'} />}
        />
      </Link>
      <Link
        className={classes.albumLink}
        to={linkToRecord(basePath, record.id, 'show')}
      >
        <span>
          <OverflowTooltip title={record.name}>
            <Typography className={classes.albumName}>{record.name}</Typography>
          </OverflowTooltip>
          {record.tags && record.tags['albumversion'] && (
            <Typography className={classes.albumVersion}>
              {record.tags['albumversion']}
            </Typography>
          )}
        </span>
      </Link>
      {showArtist ? (
        <ArtistLinkField record={record} className={classes.albumSubtitle} />
      ) : (
        <AlbumDatesField record={record} className={classes.albumSubtitle} />
      )}
    </div>
  )
}

const LoadedAlbumGrid = ({ records, basePath = '/album', width }) => {
  const classes = useStyles()
  const { filterValues } = useListContext()
  const isArtistView = !!(filterValues && filterValues.artist_id)
  return (
    <div className={classes.root}>
      <div
        className={classes.grid}
        style={{
          gridTemplateColumns: `repeat(${getColsForWidth(width)}, minmax(0, 1fr))`,
        }}
      >
        {records.map((record) => (
          <div className={classes.gridListTile} key={record.id}>
            <AlbumGridTile
              record={record}
              basePath={basePath}
              showArtist={!isArtistView}
            />
          </div>
        ))}
      </div>
    </div>
  )
}

const AlbumGridView = ({ albumListType, basePath, width }) => {
  const { data = [], isPending } = useListContext()
  const hide = isPending && albumListType === 'random'
  return hide ? (
    <Loading />
  ) : (
    <LoadedAlbumGrid records={data} basePath={basePath} width={width} />
  )
}

const AlbumGridViewWithWidth = withWidth()(AlbumGridView)

export default AlbumGridViewWithWidth
