import {
  Edit,
  FormDataConsumer,
  SimpleForm,
  TextInput,
  TextField,
  BooleanInput,
  required,
  useTranslate,
  usePermissions,
  ReferenceInput,
  SelectInput,
  useRecordContext,
} from 'react-admin'
import { isWritable, Title } from '../common'
import type { PlaylistRecord } from '../types/records'

const SyncFragment = ({
  formData,
  variant: _variant,
  ...rest
}: {
  formData: { path?: string }
  variant?: string
}) => {
  return (
    <>
      {formData.path && <BooleanInput source="sync" {...rest} />}
      {formData.path && <TextField source="path" {...rest} />}
    </>
  )
}

const PlaylistTitle = ({
  record: recordOverride,
}: {
  record?: PlaylistRecord
}) => {
  const record = useRecordContext<PlaylistRecord>({ record: recordOverride })
  const translate = useTranslate()
  const resourceName = translate('resources.playlist.name', { smart_count: 1 })
  return <Title subTitle={`${resourceName} "${record ? record.name : ''}"`} />
}

const PlaylistEditForm = ({ record }: { record?: PlaylistRecord }) => {
  const { permissions } = usePermissions()
  return (
    <SimpleForm>
      <TextInput source="name" validate={required()} />
      <TextInput
        multiline
        minRows={3}
        source="comment"
        fullWidth
        slotProps={{ htmlInput: { style: { resize: 'vertical' } } }}
      />
      {permissions === 'admin' ? (
        <ReferenceInput
          source="ownerId"
          reference="user"
          perPage={-1}
          sort={{ field: 'name', order: 'ASC' }}
        >
          <SelectInput
            label={'resources.playlist.fields.ownerName'}
            optionText="userName"
          />
        </ReferenceInput>
      ) : (
        <TextField source="ownerName" />
      )}
      <BooleanInput
        source="public"
        disabled={!isWritable(record?.ownerId)}
      />
      <FormDataConsumer>
        {(formDataProps) => <SyncFragment {...formDataProps} />}
      </FormDataConsumer>
    </SimpleForm>
  )
}

const PlaylistEdit = (props: Record<string, unknown>) => (
  <Edit title={<PlaylistTitle />} actions={false} redirect="list" {...props}>
    <PlaylistEditForm {...props} />
  </Edit>
)

export default PlaylistEdit
