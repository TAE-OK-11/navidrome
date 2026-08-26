// @ts-nocheck -- legacy JavaScript migration; remove after typing this module
import { useMemo } from 'react'
import { useNavigate } from 'react-router-dom'
import {
  Datagrid,
  DatagridBody,
  DatagridRow,
  Filter,
  FunctionField,
  NumberField,
  SearchInput,
  SelectInput,
  TextField,
  useTranslate,
  NullableBooleanInput,
  usePermissions,
} from 'react-admin'
import { useMediaQuery } from '@mui/material'
import FavoriteIcon from '@mui/icons-material/Favorite'
import FavoriteBorderIcon from '@mui/icons-material/FavoriteBorder'
import { useDrag } from 'react-dnd'
import clsx from 'clsx'
import {
  ArtistContextMenu,
  CoverArtAvatar,
  List,
  useGetHandleArtistClick,
  RatingField,
  useSelectedFields,
  useResourceRefresh,
} from '../common'
import config from '../config'
import ArtistListActions from './ArtistListActions'
import ArtistSimpleList from './ArtistSimpleList'
import { DraggableTypes } from '../consts'
import en from '../i18n/en.json'
import { formatBytes } from '../utils/index'
import { withWidth } from '../themes/useWidth'

const rowClass = 'nd-artist-grid-row'
const missingRowClass = 'nd-artist-grid-row-missing'

const ArtistFilter = (props) => {
  const translate = useTranslate()
  const { permissions } = usePermissions()
  const isAdmin = permissions === 'admin'
  const rolesObj = en?.resources?.artist?.roles
  const roles = Object.keys(rolesObj).reduce((acc, role) => {
    acc.push({
      id: role,
      name: translate(`resources.artist.roles.${role}`, {
        smart_count: 2,
      }),
    })
    return acc
  }, [])
  roles?.sort((a, b) => a.name.localeCompare(b.name))
  return (
    <Filter {...props}>
      <SearchInput id="search" source="name" alwaysOn />
      <SelectInput source="role" choices={roles} alwaysOn />
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

const ArtistDatagridRow = (props) => {
  const { record } = props
  const [, dragArtistRef] = useDrag(
    () => ({
      type: DraggableTypes.ARTIST,
      item: { artistIds: [record?.id] },
      options: { dropEffect: 'copy' },
    }),
    [record],
  )
  const computedClasses = clsx(
    props.className,
    rowClass,
    record?.missing && missingRowClass,
  )
  return (
    <DatagridRow ref={dragArtistRef} {...props} className={computedClasses} />
  )
}

const ArtistDatagridBody = (props) => (
  <DatagridBody {...props} row={<ArtistDatagridRow />} />
)

const ArtistDatagrid = ({ sx, ...props }) => (
  <Datagrid
    {...props}
    sx={[
      {
        [`& .${rowClass} td`]: {
          paddingTop: '4px !important',
          paddingBottom: '4px !important',
        },
        [`& .${rowClass}:hover`]: {
          '& .nd-artist-context-menu, & .nd-rating-field': {
            visibility: 'visible',
          },
        },
        [`& .${missingRowClass}`]: { opacity: 0.3 },
      },
      ...(Array.isArray(sx) ? sx : [sx]),
    ]}
    body={<ArtistDatagridBody />}
  />
)

const ArtistListView = ({ hasShow, hasEdit, hasList, width, ...rest }) => {
  const { filterValues } = rest
  const handleArtistLink = useGetHandleArtistClick(width)
  const navigate = useNavigate()
  const isXsmall = useMediaQuery((theme) => theme.breakpoints.down('sm'))
  useResourceRefresh('artist')

  const role = filterValues?.role
  const getCounter = (record, counter) => {
    if (!record) return undefined
    return role ? record?.stats?.[role]?.[counter] : record?.[counter]
  }
  const getAlbumCount = (record) => getCounter(record, 'albumCount')
  const getSongCount = (record) => getCounter(record, 'songCount')
  const getSize = (record) => {
    const size = getCounter(record, 'size')
    return size ? formatBytes(size) : '0 MB'
  }

  const toggleableFields = useMemo(
    () => ({
      playCount: <NumberField source="playCount" sortByOrder={'DESC'} />,
      rating: config.enableStarRating && (
        <RatingField
          source="rating"
          sortByOrder={'DESC'}
          resource={'artist'}
          sx={{ visibility: 'hidden' }}
        />
      ),
    }),
    [],
  )

  const columns = useSelectedFields({
    resource: 'artist',
    columns: toggleableFields,
  })

  return isXsmall ? (
    <ArtistSimpleList
      linkType={(id) => navigate(handleArtistLink(id))}
      {...rest}
    />
  ) : (
    <ArtistDatagrid rowClick={handleArtistLink}>
      <CoverArtAvatar source="id" />
      <TextField source="name" />
      <FunctionField
        source="albumCount"
        sortByOrder={'DESC'}
        render={getAlbumCount}
      />
      <FunctionField
        source="songCount"
        sortByOrder={'DESC'}
        render={getSongCount}
      />
      <FunctionField source="size" sortByOrder={'DESC'} render={getSize} />
      {columns}
      <ArtistContextMenu
        source={'starred_at'}
        sortByOrder={'DESC'}
        sortable={config.enableFavourites}
        className="nd-artist-context-menu"
        sx={{ visibility: 'hidden' }}
        label={
          config.enableFavourites && (
            <FavoriteBorderIcon
              fontSize={'small'}
              sx={{ ml: '3px', mt: '-2px', verticalAlign: 'text-top' }}
            />
          )
        }
      />
    </ArtistDatagrid>
  )
}

const ArtistList = (props) => {
  return (
    <>
      <List
        {...props}
        sort={{ field: 'name', order: 'ASC' }}
        exporter={false}
        bulkActionButtons={false}
        filters={<ArtistFilter />}
        filterDefaultValues={{ role: 'albumartist' }}
        actions={<ArtistListActions />}
      >
        <ArtistListView {...props} />
      </List>
    </>
  )
}

const ArtistListWithWidth = withWidth()(ArtistList)

export default ArtistListWithWidth
