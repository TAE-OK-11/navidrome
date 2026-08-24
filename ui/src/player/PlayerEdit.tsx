import {
  TextInput,
  BooleanInput,
  TextField,
  Edit,
  required,
  SimpleForm,
  SelectInput,
  ReferenceInput,
  useRecordContext,
  useTranslate,
} from 'react-admin'
import { Title } from '../common'
import config from '../config'
import { BITRATE_CHOICES } from '../consts'

const PlayerTitle = ({
  record: recordOverride,
}: {
  record?: { name?: string }
}) => {
  const record = useRecordContext({ record: recordOverride })
  const translate = useTranslate()
  const resourceName = translate('resources.player.name', { smart_count: 1 })
  return <Title subTitle={`${resourceName} ${record ? record.name : ''}`} />
}

const PlayerEdit = (props) => (
  <Edit title={<PlayerTitle />} {...props}>
    <SimpleForm>
      <TextInput source="name" validate={[required()]} />
      <ReferenceInput
        source="transcodingId"
        reference="transcoding"
        sort={{ field: 'name', order: 'ASC' }}
      >
        <SelectInput source="name" resettable />
      </ReferenceInput>
      <SelectInput source="maxBitRate" resettable choices={BITRATE_CHOICES} />
      <BooleanInput source="reportRealPath" fullWidth />
      {(config.lastFMEnabled || config.listenBrainzEnabled) && (
        <BooleanInput source="scrobbleEnabled" fullWidth />
      )}
      <TextField source="client" />
      <TextField source="userName" />
    </SimpleForm>
  </Edit>
)

export default PlayerEdit
