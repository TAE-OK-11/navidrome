// @ts-nocheck -- legacy JavaScript migration; remove after typing this module
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
  <Filter {...props} variant={'outlined'}>
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
}) => {
  const isNotSmall = useMediaQuery((theme) => theme.breakpoints.up('sm'))
  const translate = useTranslate()

  return (
    <TopToolbar className={className} {...sanitizeListRestProps(rest)}>
      {isAdmin && (
        <CreateButton basePath="/radio">
          {translate('ra.action.create')}
        </CreateButton>
      )}
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

const CoverArtField = ({ record: recordOverride }) => {
  const record = useRecordContext({ record: recordOverride })
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
    coverArt: <CoverArtField source="id" sortable={false} />,
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

  const handleRowClick = async (id, basePath, record) => {
    dispatch(setTrack(await songFromRadio(record)))
  }

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
