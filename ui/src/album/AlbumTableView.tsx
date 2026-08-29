import React, { useMemo } from 'react'
import {
  Datagrid,
  DatagridBody,
  DatagridRow,
  DateField,
  NumberField,
  TextField,
  FunctionField,
} from 'react-admin'
import { Box, useMediaQuery } from '@mui/material'
import FavoriteBorderIcon from '@mui/icons-material/FavoriteBorder'
import { useDrag } from 'react-dnd'
import {
  ArtistLinkField,
  CoverArtAvatar,
  DurationField,
  RangeField,
  SimpleList,
  AlbumContextMenu,
  RatingField,
  useSelectedFields,
  SizeField,
} from '../common'
import config from '../config'
import { DraggableTypes } from '../consts'
import type { AlbumRecord } from '../types/records'

const hoverRevealSx = {
  '&:hover .album-context-menu, &:hover .album-rating-field': {
    visibility: 'visible',
  },
}

const AlbumDatagridRow = (props: {
  record?: AlbumRecord
  className?: string
  sx?: unknown
}) => {
  const { record, className } = props
  const [, dragAlbumRef] = useDrag(
    () => ({
      type: DraggableTypes.ALBUM,
      item: { albumIds: [record?.id] },
      options: { dropEffect: 'copy' },
    }),
    [record],
  )
  return (
    <DatagridRow
      ref={dragAlbumRef as unknown as React.Ref<HTMLTableRowElement>}
      {...props}
      className={className}
      sx={[
        hoverRevealSx,
        record?.missing && { opacity: 0.3 },
        ...(Array.isArray(props.sx) ? props.sx : [props.sx]),
      ]}
    />
  )
}

const AlbumDatagridBody = (props) => (
  <DatagridBody {...props} row={<AlbumDatagridRow />} />
)

const AlbumDatagrid = (props) => (
  <Datagrid {...props} body={<AlbumDatagridBody />} />
)

const AlbumTableView = ({
  hasShow: _hasShow,
  hasEdit: _hasEdit,
  hasList: _hasList,
  syncWithLocation: _syncWithLocation,
  ...rest
}: {
  hasShow?: boolean
  hasEdit?: boolean
  hasList?: boolean
  syncWithLocation?: boolean
  [key: string]: unknown
}) => {
  const isDesktop = useMediaQuery((theme) => theme.breakpoints.up('md'))
  const isXsmall = useMediaQuery((theme) => theme.breakpoints.down('sm'))

  const toggleableFields = useMemo(() => {
    return {
      artist: <ArtistLinkField source="albumArtist" />,
      songCount: isDesktop && (
        <NumberField source="songCount" sortByOrder={'DESC'} />
      ),
      playCount: isDesktop && (
        <NumberField source="playCount" sortByOrder={'DESC'} />
      ),
      year: (
        <RangeField source={'year'} sortBy={'max_year'} sortByOrder={'DESC'} />
      ),
      mood: isDesktop && (
        <FunctionField
          source="mood"
          render={(r) => r.tags?.mood?.[0] || ''}
          sortable={false}
        />
      ),
      duration: isDesktop && <DurationField source="duration" />,
      size: isDesktop && <SizeField source="size" />,
      rating: config.enableStarRating && (
        <RatingField
          source={'rating'}
          resource={'album'}
          sortByOrder={'DESC'}
          className="album-rating-field"
          sx={{ visibility: 'hidden' }}
        />
      ),
      createdAt: isDesktop && <DateField source="createdAt" showTime />,
    }
  }, [isDesktop])

  const columns = useSelectedFields({
    resource: 'album',
    columns: toggleableFields,
    defaultOff: ['createdAt', 'size', 'mood'],
  })

  return isXsmall ? (
    <SimpleList
      primaryText={(r: AlbumRecord) => r.name ?? ''}
      secondaryText={(r: AlbumRecord) => (
        <>
          {r.albumArtist as React.ReactNode}
          {config.enableStarRating && (
            <>
              <br />
              <RatingField
                record={r}
                sortByOrder={'DESC'}
                source={'rating'}
                resource={'album'}
                size={'small'}
              />
            </>
          )}
        </>
      )}
      tertiaryText={(r: AlbumRecord) => (
        <>
          <RangeField
            record={r as Record<string, string | number | null | undefined>}
            source={'year'}
            sortBy={'max_year'}
          />
          &nbsp;&nbsp;&nbsp;&nbsp;&nbsp;
        </>
      )}
      leftIcon={(r) => (
        <Box component="span" sx={{ marginRight: '8px' }}>
          <CoverArtAvatar record={r} variant="square" />
        </Box>
      )}
      linkType={'show'}
      rightIcon={(r) => <AlbumContextMenu record={r} />}
      {...rest}
    />
  ) : (
    <AlbumDatagrid rowClick={'show'} {...rest}>
      <CoverArtAvatar source="id" variant="square" />
      <TextField source="name" />
      {columns}
      <AlbumContextMenu
        source={'starred_at'}
        sortByOrder={'DESC'}
        sortable={config.enableFavourites}
        className="album-context-menu"
        label={
          config.enableFavourites && (
            <FavoriteBorderIcon
              fontSize={'small'}
              sx={{ ml: '3px', mt: '-2px', verticalAlign: 'text-top' }}
            />
          )
        }
      />
    </AlbumDatagrid>
  )
}

export default AlbumTableView
