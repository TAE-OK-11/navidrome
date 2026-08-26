// @ts-nocheck -- legacy JavaScript migration; remove after typing this module
import { useMemo } from 'react'
import {
  AutocompleteArrayInput,
  Filter,
  FunctionField,
  NumberField,
  ReferenceArrayInput,
  SearchInput,
  TextField,
  useTranslate,
  NullableBooleanInput,
  usePermissions,
} from 'react-admin'
import { useMediaQuery } from '@mui/material'
import FavoriteIcon from '@mui/icons-material/Favorite'
import {
  DateField,
  DurationField,
  List,
  SongContextMenu,
  SongDatagrid,
  SongInfo,
  SongTitleField,
  SongSimpleList,
  RatingField,
  useResourceRefresh,
  ArtistLinkField,
  PathField,
  defaultRowsPerPageOptions,
  getStoredPerPage,
} from '../common'
import { useDispatch } from 'react-redux'
import FavoriteBorderIcon from '@mui/icons-material/FavoriteBorder'
import { setTrack } from '../actions'
import { SongListActions } from './SongListActions'
import { AlbumLinkField } from './AlbumLinkField'
import { SongBulkActions, QualityInfo, useSelectedFields } from '../common'
import config from '../config'
import ExpandInfoDialog from '../dialogs/ExpandInfoDialog'

const autocompleteSx = {
  '& .MuiChip-root': { m: 0, height: 24 },
}

const SongFilter = (props) => {
  const translate = useTranslate()
  const { permissions } = usePermissions()
  const isAdmin = permissions === 'admin'
  return (
    <Filter {...props}>
      <SearchInput source="title" alwaysOn />
      <ReferenceArrayInput
        label={translate('resources.song.fields.genre')}
        source="genre_id"
        reference="genre"
        perPage={-1}
        sort={{ field: 'name', order: 'ASC' }}
        filterToQuery={(searchText) => ({ name: [searchText] })}
      >
        <AutocompleteArrayInput emptyText="-- None --" sx={autocompleteSx} />
      </ReferenceArrayInput>
      <ReferenceArrayInput
        label={translate('resources.song.fields.grouping')}
        source="grouping"
        reference="tag"
        perPage={-1}
        sort={{ field: 'tagValue', order: 'ASC' }}
        filter={{ tag_name: 'grouping' }}
        filterToQuery={(searchText) => ({
          tag_value: [searchText],
        })}
      >
        <AutocompleteArrayInput
          emptyText="-- None --"
          sx={autocompleteSx}
          optionText="tagValue"
        />
      </ReferenceArrayInput>
      <ReferenceArrayInput
        label={translate('resources.song.fields.mood')}
        source="mood"
        reference="tag"
        perPage={-1}
        sort={{ field: 'tagValue', order: 'ASC' }}
        filter={{ tag_name: 'mood' }}
        filterToQuery={(searchText) => ({
          tag_value: [searchText],
        })}
      >
        <AutocompleteArrayInput
          emptyText="-- None --"
          sx={autocompleteSx}
          optionText="tagValue"
        />
      </ReferenceArrayInput>
      {config.enableFavourites && (
        <NullableBooleanInput
          source="starred"
          label={<FavoriteIcon fontSize={'small'} />}
        />
      )}
      {isAdmin && <NullableBooleanInput source="missing" />}
    </Filter>
  )
}

const SongList = (props) => {
  const dispatch = useDispatch()
  const isXsmall = useMediaQuery((theme) => theme.breakpoints.down('sm'))
  const isDesktop = useMediaQuery((theme) => theme.breakpoints.up('md'))
  useResourceRefresh('song')

  const handleRowClick = (id, basePath, record) => {
    dispatch(setTrack(record))
  }

  const toggleableFields = useMemo(() => {
    return {
      album: isDesktop && <AlbumLinkField source="album" sortByOrder={'ASC'} />,
      artist: <ArtistLinkField source="artist" />,
      composer: <ArtistLinkField source="composer" sortable={false} />,
      albumArtist: <ArtistLinkField source="albumArtist" />,
      trackNumber: isDesktop && (
        <NumberField source="trackNumber" sortable={false} />
      ),
      playCount: isDesktop && (
        <NumberField source="playCount" sortByOrder={'DESC'} />
      ),
      playDate: <DateField source="playDate" sortByOrder={'DESC'} showTime />,
      year: isDesktop && (
        <FunctionField
          source="year"
          render={(r) => r.year || ''}
          sortByOrder={'DESC'}
        />
      ),
      quality: isDesktop && <QualityInfo source="quality" sortable={false} />,
      channels: isDesktop && (
        <NumberField source="channels" sortByOrder={'ASC'} />
      ),
      duration: <DurationField source="duration" />,
      rating: config.enableStarRating && (
        <RatingField
          source="rating"
          sortByOrder={'DESC'}
          resource={'song'}
          sx={{ visibility: 'hidden' }}
        />
      ),
      bpm: isDesktop && <NumberField source="bpm" />,
      genre: <TextField source="genre" />,
      mood: isDesktop && (
        <FunctionField
          source="mood"
          render={(r) => r.tags?.mood?.[0] || ''}
          sortable={false}
        />
      ),
      comment: <TextField source="comment" />,
      path: <PathField source="path" />,
      createdAt: (
        <DateField source="createdAt" sortBy="recently_added" showTime />
      ),
    }
  }, [isDesktop])

  const columns = useSelectedFields({
    resource: 'song',
    columns: toggleableFields,
    defaultOff: [
      'composer',
      'channels',
      'bpm',
      'playDate',
      'albumArtist',
      'genre',
      'mood',
      'comment',
      'path',
      'createdAt',
    ],
  })

  return (
    <>
      <List
        {...props}
        sort={{ field: 'title', order: 'ASC' }}
        exporter={false}
        bulkActionButtons={<SongBulkActions />}
        actions={<SongListActions />}
        filters={<SongFilter />}
        perPage={getStoredPerPage(
          'song',
          defaultRowsPerPageOptions,
          isXsmall ? 50 : 15,
        )}
      >
        {isXsmall ? (
          <SongSimpleList />
        ) : (
          <SongDatagrid
            rowClick={handleRowClick}
            contextAlwaysVisible={!isDesktop}
          >
            <SongTitleField source="title" showTrackNumbers={false} />
            {columns}
            <SongContextMenu
              source={'starred_at'}
              sortByOrder={'DESC'}
              sortable={config.enableFavourites}
              sx={{ visibility: isDesktop ? 'hidden' : 'visible' }}
              label={
                config.enableFavourites && (
                  <FavoriteBorderIcon
                    fontSize={'small'}
                    sx={{ ml: '3px', mt: '-2px', verticalAlign: 'text-top' }}
                  />
                )
              }
            />
          </SongDatagrid>
        )}
      </List>
      <ExpandInfoDialog content={<SongInfo />} />
    </>
  )
}

export default SongList
