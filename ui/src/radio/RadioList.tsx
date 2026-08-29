import { Avatar, useMediaQuery } from '@mui/material'
import React, { cloneElement } from 'react'
import {
  CreateButton,
  Datagrid,
  DateField,
  EditButton,
  Filter,
  sanitizeListRestProps,
  SearchInput,
  SimpleList,
  TextField,
  TopToolbar,
  UrlField,
  useTranslate,
  useRecordContext,
  type ListActionsProps,
  type RaRecord,
  type Identifier,
  type RowClickFunction,
} from 'react-admin'
import {
  List,
  defaultRowsPerPageOptions,
  getStoredPerPage,
  useImageUrl,
  ToggleFieldsMenu,
  useSelectedFields,
} from '../common'
import subsonic from '../subsonic'
import { StreamField } from './StreamField'
import { setTrack } from '../actions'
import { songFromRadio } from './helper'
import { RADIO_PLACEHOLDER_IMAGE } from '../consts'
import { useDispatch } from 'react-redux'

const RadioFilter = (props) => (
  <Filter {...props}>
    <SearchInput id="search" source="name" alwaysOn />
  </Filter>
)

const RadioListActions = ({
  className,
  filters,
  resource,
  showFilter,
  displayedFilters,
  filterValues,
  isAdmin,
  ...rest
}: ListActionsProps & { isAdmin?: boolean }) => {
  const isNotSmall = useMediaQuery((theme) => theme.breakpoints.up('sm'))
  const translate = useTranslate()

  return (
    <TopToolbar className={className} {...sanitizeListRestProps(rest)}>
      {isAdmin && <CreateButton>{translate('ra.action.create')}</CreateButton>}
      {filters &&
        cloneElement(filters, {
          resource,
          showFilter,
          displayedFilters,
          filterValues,
          context: 'button',
        })}
      {isNotSmall && <ToggleFieldsMenu resource="radio" />}
    </TopToolbar>
  )
}

type CoverArtRecord = RaRecord<Identifier> & {
  uploadedImage?: boolean
  name?: string
}

const CoverArtField = ({
  record: recordOverride,
}: {
  record?: CoverArtRecord
}) => {
  const record = useRecordContext<CoverArtRecord>({ record: recordOverride })
  const directUrl = record?.uploadedImage
    ? subsonic.getCoverArtUrl(record, 40, true)
    : null
  const { imgUrl } = useImageUrl(directUrl)
  if (!record) return null
  const src = imgUrl || RADIO_PLACEHOLDER_IMAGE
  return (
    <Avatar
      src={src}
      variant="rounded"
      sx={{ width: 40, height: 40 }}
      alt={record.name}
    />
  )
}
CoverArtField.defaultProps = { label: '' }

const RadioList = ({ permissions, ...props }) => {
  const isXsmall = useMediaQuery((theme) => theme.breakpoints.down('sm'))
  const dispatch = useDispatch()
  const isAdmin = permissions === 'admin'

  const toggleableFields = {
    coverArt: <CoverArtField />,
    name: <TextField source="name" />,
    homePageUrl: (
      <UrlField
        source="homePageUrl"
        onClick={(e) => e.stopPropagation()}
        target="_blank"
        rel="noopener noreferrer"
      />
    ),
    streamUrl: <TextField source="streamUrl" />,
    updatedAt: <DateField source="updatedAt" showTime />,
    createdAt: <DateField source="createdAt" showTime />,
  }

  const columns = useSelectedFields({
    resource: 'radio',
    columns: toggleableFields,
    defaultOff: ['streamUrl', 'createdAt'],
  })

  const handleRowClick = ((_id, _resource, record) => {
    void songFromRadio(record).then((track) => dispatch(setTrack(track)))
    return false
  }) as RowClickFunction

  return (
    <List
      {...props}
      exporter={false}
      sort={{ field: 'name', order: 'ASC' }}
      bulkActionButtons={isAdmin ? undefined : false}
      hasCreate={isAdmin}
      actions={<RadioListActions isAdmin={isAdmin} />}
      filters={<RadioFilter />}
      perPage={getStoredPerPage(
        'radio',
        defaultRowsPerPageOptions,
        isXsmall ? 25 : 10,
      )}
    >
      {isXsmall ? (
        <SimpleList
          leftAvatar={(r) => <CoverArtField record={r} />}
          leftIcon={(r) => (
            <StreamField
              record={r}
              source={'streamUrl'}
              hideUrl
              onClick={(e) => {
                e.preventDefault()
                e.stopPropagation()
              }}
            />
          )}
          primaryText={(r) => r.name}
          secondaryText={(r) => r.homePageUrl}
        />
      ) : (
        <Datagrid rowClick={handleRowClick}>
          {columns}
          {isAdmin && <EditButton />}
        </Datagrid>
      )}
    </List>
  )
}

export default RadioList
