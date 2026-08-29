import { cloneElement, type ReactElement } from 'react'
import { useSelector } from 'react-redux'
import { Navigate, useLocation } from 'react-router-dom'
import {
  AutocompleteArrayInput,
  AutocompleteInput,
  Filter,
  NullableBooleanInput,
  NumberInput,
  ReferenceArrayInput,
  ReferenceInput,
  SearchInput,
  useListContext,
  usePermissions,
  useRefresh,
  useTranslate,
} from 'react-admin'
import FavoriteIcon from '@mui/icons-material/Favorite'
import {
  List,
  Pagination,
  Title,
  useAlbumsPerPage,
  useResourceRefresh,
  useScrollRestoration,
  useSetToggleableFields,
} from '../common'
import AlbumListActions from './AlbumListActions'
import AlbumTableView from './AlbumTableView'
import AlbumGridView from './AlbumGridView'
import albumLists from './albumLists'
import {
  getStoredDefaultView,
  isResourceDefaultView,
} from '../personal/defaultViews'
import config from '../config'
import AlbumInfo from './AlbumInfo'
import ExpandInfoDialog from '../dialogs/ExpandInfoDialog'
import { humanize } from 'inflection'
import { withWidth, type Width } from '../themes/useWidth'
import type { AppState } from '../types/redux'

// Waits for rows: restoring into an unrendered list leaves the page too short to hold the offset.
const ScrollRestorer = ({ children, ...rest }: { children: ReactElement }) => {
  const { isPending, total } = useListContext()
  useScrollRestoration(!isPending && (total ?? 0) > 0)
  return cloneElement(children, rest)
}

const autocompleteSx = {
  '& .MuiChip-root': { m: 0, height: 24 },
}

const formatReleaseType = (record) =>
  record?.tagValue ? humanize(record?.tagValue) : '-- None --'

const AlbumFilter = (props) => {
  const translate = useTranslate()
  const { permissions } = usePermissions()
  const isAdmin = permissions === 'admin'
  return (
    <Filter {...props}>
      <SearchInput id="search" source="name" alwaysOn />
      <ReferenceInput
        label={translate('resources.album.fields.artist')}
        source="artist_id"
        reference="artist"
        sort={{ field: 'name', order: 'ASC' }}
        filterToQuery={(searchText) => ({ name: [searchText] })}
      >
        <AutocompleteInput emptyText="-- None --" />
      </ReferenceInput>
      <ReferenceArrayInput
        label={translate('resources.album.fields.genre')}
        source="genre_id"
        reference="genre"
        perPage={-1}
        sort={{ field: 'name', order: 'ASC' }}
        filterToQuery={(searchText) => ({ name: [searchText] })}
      >
        <AutocompleteArrayInput emptyText="-- None --" sx={autocompleteSx} />
      </ReferenceArrayInput>
      <ReferenceInput
        label={translate('resources.album.fields.recordLabel')}
        source="recordlabel"
        reference="tag"
        perPage={-1}
        sort={{ field: 'tagValue', order: 'ASC' }}
        filter={{ tag_name: 'recordlabel' }}
        filterToQuery={(searchText) => ({
          tag_value: [searchText],
        })}
      >
        <AutocompleteInput emptyText="-- None --" optionText="tagValue" />
      </ReferenceInput>
      <ReferenceArrayInput
        label={translate('resources.album.fields.grouping')}
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
        label={translate('resources.album.fields.mood')}
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
      <ReferenceInput
        label={translate('resources.album.fields.media')}
        source="media"
        reference="tag"
        perPage={-1}
        sort={{ field: 'tagValue', order: 'ASC' }}
        filter={{ tag_name: 'media' }}
        filterToQuery={(searchText) => ({
          tag_value: [searchText],
        })}
      >
        <AutocompleteInput emptyText="-- None --" optionText="tagValue" />
      </ReferenceInput>
      <ReferenceInput
        label={translate('resources.album.fields.releaseType')}
        source="releasetype"
        reference="tag"
        perPage={-1}
        sort={{ field: 'tagValue', order: 'ASC' }}
        filter={{ tag_name: 'releasetype' }}
        filterToQuery={(searchText) => ({
          tag_value: [searchText],
        })}
      >
        <AutocompleteInput
          emptyText="-- None --"
          optionText={formatReleaseType}
        />
      </ReferenceInput>
      <NullableBooleanInput source="compilation" />
      <NumberInput source="year" />
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

const AlbumListTitle = ({ albumListType }) => {
  const translate = useTranslate()
  let title = translate('resources.album.name', { smart_count: 2 })
  if (albumListType) {
    let listTitle = translate(`resources.album.lists.${albumListType}`, {
      smart_count: 2,
    })
    title = `${title} - ${listTitle}`
  }
  return <Title subTitle={title} args={{ smart_count: 2 }} />
}

const AlbumListPagination = ({ albumListType, ...rest }) => {
  const { isPending } = useListContext()
  if (isPending && albumListType === 'random') {
    return null
  }
  return <Pagination {...rest} />
}

const randomStartingSeed = Math.random().toString()

const AlbumList = (props: { width?: Width }) => {
  const { width } = props
  const albumView = useSelector((state: AppState) => state.albumView)
  const [perPage, perPageOptions] = useAlbumsPerPage(width)
  const location = useLocation()
  const refreshVersion = useSelector(
    (state: AppState) => state.activity?.refresh?.lastReceived || 0,
  )
  const refresh = useRefresh()
  useResourceRefresh('album')

  const seed = `${randomStartingSeed}-${refreshVersion}`

  const albumListType = location.pathname
    .replace(/^\/album/, '')
    .replace(/^\//, '')

  // Workaround to force album columns to appear the first time.
  // See https://github.com/navidrome/navidrome/pull/923#issuecomment-833004842
  // TODO: Find a better solution
  useSetToggleableFields(
    'album',
    [
      'artist',
      'songCount',
      'playCount',
      'year',
      'mood',
      'duration',
      'rating',
      'size',
      'createdAt',
    ],
    ['createdAt', 'size'],
  )

  // If it does not have filter/sort params (usually coming from Menu),
  // reload with correct filter/sort params
  if (!location.search) {
    const type = albumListType || getStoredDefaultView()
    if (isResourceDefaultView(type)) {
      return <Navigate to={`/${type}`} replace />
    }
    const listParams = albumLists[type]
    if (type === 'random') {
      refresh()
    }
    if (listParams) {
      return <Navigate to={`/album/${type}?${listParams.params}`} replace />
    }
  }

  return (
    <>
      <List
        {...props}
        exporter={false}
        bulkActionButtons={false}
        filter={{ seed }}
        actions={<AlbumListActions />}
        filters={<AlbumFilter />}
        perPage={perPage}
        pagination={
          <AlbumListPagination
            rowsPerPageOptions={perPageOptions}
            albumListType={albumListType}
          />
        }
        title={<AlbumListTitle albumListType={albumListType} />}
      >
        <ScrollRestorer>
          {albumView.grid ? (
            <AlbumGridView
              albumListType={albumListType}
              {...(props as Record<string, unknown>)}
            />
          ) : (
            <AlbumTableView {...(props as Record<string, unknown>)} />
          )}
        </ScrollRestorer>
      </List>
      <ExpandInfoDialog content={<AlbumInfo />} />
    </>
  )
}

const AlbumListWithWidth = withWidth()(AlbumList)

export default AlbumListWithWidth
