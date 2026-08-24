// @ts-nocheck -- legacy JavaScript migration; remove after typing this module
import React from 'react'
import {
  Datagrid,
  TextField,
  DateField,
  FunctionField,
  ReferenceField,
  Filter,
  SearchInput,
} from 'react-admin'
import { useMediaQuery } from '@mui/material'
import { SimpleList, List } from '../common'

const PlayerFilter = (props) => (
  <Filter {...props} variant={'outlined'}>
    <SearchInput id="search" source="name" alwaysOn />
  </Filter>
)

const PlayerList = ({ permissions, ...props }) => {
  const isXsmall = useMediaQuery((theme) => theme.breakpoints.down('sm'))
  return (
    <List
      {...props}
      sort={{ field: 'lastSeen', order: 'DESC' }}
      exporter={false}
      filters={<PlayerFilter />}
    >
      {isXsmall ? (
        <SimpleList
          primaryText={(r) => r.name}
          secondaryText={(r) => r.userName}
          tertiaryText={(r) => (r.maxBitRate ? r.maxBitRate : '-')}
        />
      ) : (
        <Datagrid rowClick="edit">
          <TextField source="name" />
          {permissions === 'admin' && <TextField source="userName" />}
          <ReferenceField source="transcodingId" reference="transcoding">
            <TextField source="name" />
          </ReferenceField>
          <FunctionField
            source="maxBitRate"
            render={(r) => (r.maxBitRate ? r.maxBitRate : '-')}
          />
          <DateField source="lastSeen" showTime sortByOrder={'DESC'} />
        </Datagrid>
      )}
    </List>
  )
}

export default PlayerList
