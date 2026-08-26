// @ts-nocheck -- legacy JavaScript migration; remove after typing this module
import React, { useMemo } from 'react'
import {
  Datagrid,
  DateField,
  EditButton,
  Filter,
  NullableBooleanInput,
  NumberField,
  ReferenceInput,
  SearchInput,
  SelectInput,
  TextField,
  useUpdate,
  useNotify,
  useRecordContext,
  BulkDeleteButton,
  usePermissions,
} from 'react-admin'
import Switch from '@mui/material/Switch'
import { useMediaQuery } from '@mui/material'
import {
  CoverArtAvatar,
  DurationField,
  List,
  LoveButton,
  Writable,
  isWritable,
  useSelectedFields,
  useResourceRefresh,
} from '../common'
import FavoriteIcon from '@mui/icons-material/Favorite'
import config from '../config'
import PlaylistListActions from './PlaylistListActions'
import ChangePublicStatusButton from './ChangePublicStatusButton'

const bulkButtonSx = {
  color: (theme) => (theme.palette.mode === 'dark' ? 'white' : undefined),
}

const PlaylistFilter = (props) => {
  const { permissions } = usePermissions()
  return (
    <Filter {...props}>
      <SearchInput source="q" alwaysOn />
      {permissions === 'admin' && (
        <ReferenceInput
          source="owner_id"
          label={'resources.playlist.fields.ownerName'}
          reference="user"
          perPage={-1}
          sort={{ field: 'name', order: 'ASC' }}
          alwaysOn
        >
          <SelectInput optionText="name" />
        </ReferenceInput>
      )}
      {config.enableFavourites && (
        <NullableBooleanInput
          source="starred"
          label={<FavoriteIcon fontSize={'small'} />}
        />
      )}
    </Filter>
  )
}

const TogglePublicInput = ({ resource, source }) => {
  const record = useRecordContext()
  const notify = useNotify()
  const [togglePublic] = useUpdate(
    resource,
    {
      id: record.id,
      data: { ...record, public: !record.public },
      previousData: record,
    },
    {
      mutationMode: 'pessimistic',
      onError: () => {
        notify('ra.page.error', { type: 'warning' })
      },
    },
  )

  const handleClick = (e) => {
    togglePublic()
    e.stopPropagation()
  }

  return (
    <Switch
      checked={record[source]}
      onClick={handleClick}
      disabled={!isWritable(record.ownerId)}
    />
  )
}

const ToggleAutoImport = ({ resource, source }) => {
  const record = useRecordContext()
  const notify = useNotify()
  const [ToggleAutoImport] = useUpdate(
    resource,
    {
      id: record.id,
      data: { ...record, sync: !record.sync },
      previousData: record,
    },
    {
      mutationMode: 'pessimistic',
      onError: () => {
        notify('ra.page.error', { type: 'warning' })
      },
    },
  )
  const handleClick = (e) => {
    ToggleAutoImport()
    e.stopPropagation()
  }

  return record.path ? (
    <Switch
      checked={record[source]}
      onClick={handleClick}
      disabled={!isWritable(record.ownerId)}
    />
  ) : null
}

const PlaylistListBulkActions = (props) => {
  return (
    <>
      <ChangePublicStatusButton public={true} {...props} sx={bulkButtonSx} />
      <ChangePublicStatusButton public={false} {...props} sx={bulkButtonSx} />
      <BulkDeleteButton {...props} sx={bulkButtonSx} />
    </>
  )
}

// Datagrid reads `source`/`sortable`/`label` off this element for the column
// header; only record/resource are forwarded so they never leak onto the button.
export const PlaylistLove = ({ record: recordOverride, className }) => {
  const record = useRecordContext({ record: recordOverride })
  return (
    <LoveButton record={record} resource="playlist" className={className} />
  )
}
PlaylistLove.defaultProps = { source: 'starred', sortable: false }

const PlaylistList = (props) => {
  const isXsmall = useMediaQuery((theme) => theme.breakpoints.down('sm'))
  const isDesktop = useMediaQuery((theme) => theme.breakpoints.up('md'))
  useResourceRefresh('playlist')

  const toggleableFields = useMemo(
    () => ({
      ownerName: isDesktop && <TextField source="ownerName" />,
      songCount: !isXsmall && <NumberField source="songCount" />,
      duration: <DurationField source="duration" />,
      updatedAt: isDesktop && (
        <DateField source="updatedAt" sortByOrder={'DESC'} />
      ),
      public: !isXsmall && (
        <TogglePublicInput source="public" sortByOrder={'DESC'} />
      ),
      comment: <TextField source="comment" />,
      sync: !isXsmall && (
        <ToggleAutoImport source="sync" sortByOrder={'DESC'} />
      ),
      starred: config.enableFavourites && <PlaylistLove />,
    }),
    [isDesktop, isXsmall],
  )

  const columns = useSelectedFields({
    resource: 'playlist',
    columns: toggleableFields,
    defaultOff: ['comment'],
  })

  return (
    <List
      {...props}
      exporter={false}
      sort={{ field: 'name', order: 'ASC' }}
      filters={<PlaylistFilter />}
      actions={<PlaylistListActions />}
      bulkActionButtons={!isXsmall && <PlaylistListBulkActions />}
    >
      <Datagrid rowClick="show" isRowSelectable={(r) => isWritable(r?.ownerId)}>
        <CoverArtAvatar source="id" variant="square" />
        <TextField source="name" />
        {columns}
        <Writable>
          <EditButton />
        </Writable>
      </Datagrid>
    </List>
  )
}

export default PlaylistList
