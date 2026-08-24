// @ts-nocheck -- legacy JavaScript migration; remove after typing this module
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
import { AlbumDatesField } from './AlbumDatesField'
import { withWidth } from '../themes/useWidth'

const useStyles = makeStyles(
  (theme) => ({
    root: {
      margin: 'clamp(12px, 2vw, 24px)',
      minWidth: 0,
    },
    grid: {
      display: 'grid',
      gap: 'clamp(12px, 1.8vw, 22px)',
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
      borderRadius: '0 0 12px 12px',
    },
    tileBarMobile: {
      textAlign: 'left',
      background:
        'linear-gradient(to top, rgba(0,0,0,0.7) 0%,rgba(0,0,0,0.4) 70%,rgba(0,0,0,0) 100%)',
      borderRadius: '0 0 12px 12px',
    },
    albumArtistName: {
      whiteSpace: 'nowrap',
      overflow: 'hidden',
      textOverflow: 'ellipsis',
      textAlign: 'left',
      fontSize: '1em',
    },
    albumName: {
      marginTop: theme.spacing(1),
      fontSize: '0.95rem',
      fontWeight: 700,
      lineHeight: 1.35,
      color: theme.palette.text.primary,
      overflow: 'hidden',
      whiteSpace: 'nowrap',
      textOverflow: 'ellipsis',
    },
    missingAlbum: {
      opacity: 0.3,
    },
    albumVersion: {
      fontSize: '12px',
      color: theme.palette.text.secondary,
      overflow: 'hidden',
      whiteSpace: 'nowrap',
      textOverflow: 'ellipsis',
    },
    albumSubtitle: {
      fontSize: '12px',
      color: theme.palette.text.secondary,
      overflow: 'hidden',
      whiteSpace: 'nowrap',
      textOverflow: 'ellipsis',
    },
    link: {
      position: 'relative',
      display: 'block',
      textDecoration: 'none',
      overflow: 'hidden',
      borderRadius: 12,
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
      padding: theme.spacing(1),
      border: `1px solid ${theme.palette.divider}`,
      borderRadius: 16,
      backgroundColor: theme.palette.background.paper,
      boxShadow: '0 6px 20px rgba(0, 0, 0, 0.1)',
      transition: 'transform 180ms ease, box-shadow 180ms ease',
      '&:hover': {
        transform: 'translateY(-3px)',
        boxShadow: '0 12px 28px rgba(0, 0, 0, 0.18)',
      },
    },
    albumPlayButton: { color: 'white' },
  }),
  { name: 'NDAlbumGridView' },
)

const useCoverStyles = makeStyles((theme) => ({
  coverContainer: {
    width: '100%',
    aspectRatio: '1 / 1',
    overflow: 'hidden',
    borderRadius: '12px',
    backgroundColor: theme.palette.action.hover,
  },
  coverContent: {
    width: '100%',
    height: '100%',
  },
  cover: {
    display: 'block',
    width: '100%',
    height: '100%',
    objectFit: 'cover',
    transition: 'opacity 0.3s ease-in-out, transform 250ms ease',
    '&:hover': {
      transform: 'scale(1.025)',
    },
  },
  coverLoading: {
    opacity: 0,
  },
}))

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
