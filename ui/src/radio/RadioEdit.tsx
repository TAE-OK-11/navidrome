// @ts-nocheck -- legacy JavaScript migration; remove after typing this module
import {
  DateField,
  Edit,
  required,
  SimpleForm,
  TextInput,
  useRecordContext,
  useTranslate,
} from 'react-admin'
import { Box, CardMedia } from '@mui/material'
import { urlValidate } from '../utils/validations'
import { Title, ImageUploadOverlay, useImageLoadingState } from '../common'
import subsonic from '../subsonic'
import config from '../config'
import { RADIO_PLACEHOLDER_IMAGE } from '../consts'

const RadioTitle = ({ record: recordOverride }) => {
  const record = useRecordContext({ record: recordOverride })
  const translate = useTranslate()
  const resourceName = translate('resources.radio.name', {
    smart_count: 1,
  })
  return <Title subTitle={`${resourceName} ${record ? record.name : ''}`} />
}

const RadioEdit = (props) => {
  return (
    <Edit title={<RadioTitle />} {...props}>
      <SimpleForm variant="outlined" {...props}>
        <RadioCoverArt />
        <TextInput source="name" validate={[required()]} />
        <TextInput
          type="url"
          source="streamUrl"
          fullWidth
          validate={[required(), urlValidate]}
        />
        <TextInput
          type="url"
          source="homePageUrl"
          fullWidth
          validate={[urlValidate]}
        />
        <DateField variant="body1" source="updatedAt" showTime />
        <DateField variant="body1" source="createdAt" showTime />
      </SimpleForm>
    </Edit>
  )
}

const RadioCoverArt = ({ record: recordOverride }) => {
  const record = useRecordContext({ record: recordOverride })
  const { imageLoading, handleImageLoad, handleImageError } =
    useImageLoadingState(record?.id)

  if (!record) return null

  return (
    <Box
      sx={{
        display: 'inline-flex',
        position: 'relative',
        width: '8rem',
        height: '8rem',
        mb: '1em',
      }}
    >
      {record.uploadedImage ? (
        <CardMedia
          component="img"
          src={subsonic.getCoverArtUrl(record, config.uiCoverArtSize, true)}
          sx={{
            width: '8rem',
            height: '8rem',
            objectFit: 'cover',
            cursor: 'pointer',
            transition: 'opacity 0.3s ease-in-out',
            opacity: imageLoading ? 0.5 : 1,
          }}
          onLoad={handleImageLoad}
          onError={handleImageError}
          title={record.name}
          alt={record.name}
        />
      ) : (
        <Box
          component="img"
          src={RADIO_PLACEHOLDER_IMAGE}
          sx={{ width: '8rem', height: '8rem', objectFit: 'contain' }}
          alt={record.name}
        />
      )}
      <ImageUploadOverlay
        entityType="radio"
        entityId={record.id}
        hasUploadedImage={!!record.uploadedImage}
      />
    </Box>
  )
}

export default RadioEdit
