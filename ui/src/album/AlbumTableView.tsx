// @ts-nocheck -- legacy JavaScript migration; remove after typing this module
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
import { useMediaQuery } from '@mui/material'
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

const hoverRevealSx = {
  '&:hover .album-context-menu, &:hover .album-rating-field': {
    visibility: 'visible',
  },
}

const AlbumDatagridRow = (props) => {
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
      ref={dragAlbumRef}
      {...props}
      className={className}
      sx={[
        hoverRevealSx,
        record.missing && { opacity: 0.3 },
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
  hasShow,
  hasEdit,
  hasList,
  syncWithLocation,
  ...rest
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
      primaryText={(r) => r.name}
      secondaryText={(r) => (
        <>
          {r.albumArtist}
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
      tertiaryText={(r) => (
        <>
          <RangeField record={r} source={'year'} sortBy={'max_year'} />
          &nbsp;&nbsp;&nbsp;&nbsp;&nbsp;
        </>
      )}
      leftIcon={(r) => (
        <span style={{ marginRight: '8px' }}>
          <CoverArtAvatar record={r} variant="square" />
        </span>
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
