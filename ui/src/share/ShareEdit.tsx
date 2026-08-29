import {
  DateTimeInput,
  BooleanInput,
  Edit,
  NumberField,
  SimpleForm,
  TextInput,
} from 'react-admin'
import { sharePlayerUrl } from '../utils'
import { Link as MuiLink } from '@mui/material'
import { DateField } from '../common'
import config from '../config'

export const ShareEdit = (props) => {
  const { id, basePath, hasCreate, ...rest } = props
  const url = sharePlayerUrl(id)
  return (
    <Edit {...props}>
      <SimpleForm {...rest}>
        <MuiLink href={url} target="_blank" rel="noopener noreferrer">
          {url}
        </MuiLink>
        <TextInput source="description" />
        {config.enableDownloads && <BooleanInput source="downloadable" />}
        <DateTimeInput source="expiresAt" />
        <TextInput source="contents" disabled />
        <TextInput source="format" disabled />
        <TextInput source="maxBitRate" disabled />
        <TextInput source="username" disabled />
        <NumberField source="visitCount" />
        <DateField source="lastVisitedAt" disabled showTime />
        <DateField source="createdAt" disabled showTime />
      </SimpleForm>
    </Edit>
  )
}
