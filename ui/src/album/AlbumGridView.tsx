// @ts-nocheck -- legacy JavaScript migration; remove after typing this module
import React from 'react'
import { Box, Typography, ImageListItemBar, useMediaQuery } from '@mui/material'
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
import { AlbumDatesField } from './AlbumDatesField'
import { withWidth } from '../themes/useWidth'
import { componentStyleOverride } from '../themes/componentStyleOverride'

const albumTextSx = (slot) => (theme) => ({
  fontSize: 12,
  color: theme.palette.text.secondary,
  overflow: 'hidden',
  whiteSpace: 'nowrap',
  textOverflow: 'ellipsis',
  ...componentStyleOverride(theme, 'NDAlbumGridView', slot),
})

const getColsForWidth = (width) => {
  if (width === 'xs') return 2
  if (width === 'sm') return 3
  if (width === 'md') return 4
  if (width === 'lg') return 6
  return 9
}

const Cover = ({ record }) => {
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
    <Box
      sx={(theme) => ({
        width: '100%',
        aspectRatio: '1 / 1',
        overflow: 'hidden',
        borderRadius: 3,
        backgroundColor: theme.palette.action.hover,
      })}
    >
      <Box ref={dragAlbumRef} sx={{ width: '100%', height: '100%' }}>
        <Box
          component="img"
          src={imgUrl || undefined}
          alt={record.name}
          sx={{
            display: 'block',
            width: '100%',
            height: '100%',
            objectFit: 'cover',
            opacity: imageLoading ? 0 : 1,
            transition: 'opacity 0.3s ease-in-out, transform 250ms ease',
            '&:hover': { transform: 'scale(1.025)' },
          }}
        />
      </Box>
    </Box>
  )
}

const AlbumGridTile = ({ showArtist, record, basePath, ...props }) => {
  const isDesktop = useMediaQuery((theme) => theme.breakpoints.up('md'), {
    noSsr: true,
  })
  if (!record) {
    return null
  }
  return (
    <Box
      sx={(theme) => ({
        minWidth: 0,
        width: '100%',
        p: 1,
        border: `1px solid ${theme.palette.divider}`,
        borderRadius: 4,
        backgroundColor: theme.palette.background.paper,
        boxShadow: '0 6px 20px rgba(0, 0, 0, 0.1)',
        transition: 'transform 180ms ease, box-shadow 180ms ease',
        opacity: record.missing ? 0.3 : 1,
        '&:hover': {
          transform: 'translateY(-3px)',
          boxShadow: '0 12px 28px rgba(0, 0, 0, 0.18)',
        },
        '& .nd-album-play-button': {
          color: 'white',
          ...componentStyleOverride(
            theme,
            'NDAlbumGridView',
            'albumPlayButton',
          ),
        },
        '& .nd-album-subtitle': albumTextSx('albumSubtitle')(theme),
        ...componentStyleOverride(theme, 'NDAlbumGridView', 'albumContainer'),
      })}
    >
      <Box
        component={Link}
        to={linkToRecord(basePath, record.id, 'show')}
        sx={{
          position: 'relative',
          display: 'block',
          textDecoration: 'none',
          overflow: 'hidden',
          borderRadius: 3,
          '&:hover .nd-album-tile-bar, &:focus-within .nd-album-tile-bar': {
            opacity: 1,
            pointerEvents: 'auto',
          },
        }}
      >
        <Cover record={record} />
        <ImageListItemBar
          className="nd-album-tile-bar"
          sx={(theme) => ({
            transition: isDesktop ? 'all 150ms ease-out' : undefined,
            opacity: isDesktop ? 0 : undefined,
            pointerEvents: isDesktop ? 'none' : undefined,
            textAlign: 'left',
            background:
              'linear-gradient(to top, rgba(0,0,0,0.7) 0%,rgba(0,0,0,0.4) 70%,rgba(0,0,0,0) 100%)',
            borderRadius: '0 0 12px 12px',
            ...componentStyleOverride(
              theme,
              'NDAlbumGridView',
              isDesktop ? 'tileBar' : 'tileBarMobile',
            ),
          })}
          subtitle={
            !record.missing && (
              <PlayButton
                className="nd-album-play-button"
                record={record}
                size="small"
              />
            )
          }
          actionIcon={<AlbumContextMenu record={record} color={'white'} />}
        />
      </Box>
      <Box
        component={Link}
        to={linkToRecord(basePath, record.id, 'show')}
        sx={(theme) => ({
          position: 'relative',
          display: 'block',
          textDecoration: 'none',
          ...componentStyleOverride(theme, 'NDAlbumGridView', 'albumLink'),
        })}
      >
        <span>
          <OverflowTooltip title={record.name}>
            <Typography
              sx={(theme) => ({
                mt: 1,
                fontSize: '0.95rem',
                fontWeight: 700,
                lineHeight: 1.35,
                color: theme.palette.text.primary,
                overflow: 'hidden',
                whiteSpace: 'nowrap',
                textOverflow: 'ellipsis',
                ...componentStyleOverride(
                  theme,
                  'NDAlbumGridView',
                  'albumName',
                ),
              })}
            >
              {record.name}
            </Typography>
          </OverflowTooltip>
          {record.tags && record.tags['albumversion'] && (
            <Typography sx={albumTextSx('albumVersion')}>
              {record.tags['albumversion']}
            </Typography>
          )}
        </span>
      </Box>
      {showArtist ? (
        <ArtistLinkField record={record} className="nd-album-subtitle" />
      ) : (
        <AlbumDatesField record={record} className="nd-album-subtitle" />
      )}
    </Box>
  )
}

const LoadedAlbumGrid = ({ records, basePath = '/album', width }) => {
  const { filterValues } = useListContext()
  const isArtistView = !!(filterValues && filterValues.artist_id)
  return (
    <Box
      sx={(theme) => ({
        m: 'clamp(12px, 2vw, 24px)',
        minWidth: 0,
        ...componentStyleOverride(theme, 'NDAlbumGridView', 'root'),
      })}
    >
      <Box
        sx={(theme) => ({
          display: 'grid',
          gap: 'clamp(12px, 1.8vw, 22px)',
          minWidth: 0,
          width: '100%',
          gridTemplateColumns: `repeat(${getColsForWidth(width)}, minmax(0, 1fr))`,
          ...componentStyleOverride(theme, 'NDAlbumGridView', 'grid'),
        })}
      >
        {records.map((record) => (
          <Box sx={{ minWidth: 0, width: '100%' }} key={record.id}>
            <AlbumGridTile
              record={record}
              basePath={basePath}
              showArtist={!isArtistView}
            />
          </Box>
        ))}
      </Box>
    </Box>
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
