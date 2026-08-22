import React, { useCallback } from 'react'
import {
  Edit,
  SimpleForm,
  TextInput,
  BooleanInput,
  required,
  SaveButton,
  DateField,
  useTranslate,
  useDataProvider,
  useNotify,
  useRedirect,
  Toolbar,
  useRecordContext,
} from 'react-admin'
import { Typography, Box } from '@mui/material'
import makeStyles from '../themes/makeStyles'
import DeleteLibraryButton from './DeleteLibraryButton'
import { Title } from '../common'
import { formatBytes, formatDuration2 } from '../utils/index'

const useStyles = makeStyles({
  toolbar: {
    display: 'flex',
    justifyContent: 'space-between',
  },
})

const LibraryTitle = ({ record }) => {
  const translate = useTranslate()
  const resourceName = translate('resources.library.name', { smart_count: 1 })
  return (
    <Title subTitle={`${resourceName} ${record ? `"${record.name}"` : ''}`} />
  )
}

const CustomToolbar = ({ showDelete }) => {
  const classes = useStyles()
  const record = useRecordContext()
  return (
    <Toolbar className={classes.toolbar}>
      <SaveButton />
      {showDelete && (
        <DeleteLibraryButton
          record={record}
          resource="library"
          basePath="/library"
        />
      )}
    </Toolbar>
  )
}

const LibraryEdit = (props) => {
  const translate = useTranslate()
  const dataProvider = useDataProvider()
  const notify = useNotify()
  const redirect = useRedirect()

  // Library ID 1 is protected (main library)
  const canDelete = props.id !== '1'
  const canEditPath = props.id !== '1'

  const save = useCallback(
    async (values) => {
      try {
        await dataProvider.update('library', {
          id: values.id,
          data: values,
          previousData: values,
        })
        notify('resources.library.notifications.updated', 'info', {
          smart_count: 1,
        })
        redirect('/library')
      } catch (error) {
        if (error.body && error.body.errors) {
          return error.body.errors
        }
      }
    },
    [dataProvider, notify, redirect],
  )

  return (
    <Edit title={<LibraryTitle />} undoable={false} {...props}>
      <SimpleForm
        {...props}
        save={save}
        toolbar={<CustomToolbar showDelete={canDelete} />}
      >
        <Box p="1em" maxWidth="800px">
          <Box display="flex">
            <Box flex={1} mr="1em">
              {/* Basic Information */}
              <Typography variant="h6" gutterBottom>
                {translate('resources.library.sections.basic')}
              </Typography>

              <TextInput
                source="name"
                label={translate('resources.library.fields.name')}
                validate={[required()]}
                variant="outlined"
              />
              <TextInput
                source="path"
                label={translate('resources.library.fields.path')}
                validate={[required()]}
                fullWidth
                variant="outlined"
                InputProps={{ readOnly: !canEditPath }} // Disable editing path for library 1
              />
              <BooleanInput
                source="defaultNewUsers"
                label={translate('resources.library.fields.defaultNewUsers')}
                variant="outlined"
              />

              <Box mt="2em" />

              {/* Statistics - Two Column Layout */}
              <Typography variant="h6" gutterBottom>
                {translate('resources.library.sections.statistics')}
              </Typography>

              <Box display="flex">
                <Box flex={1} mr="0.5em">
                  <TextInput
                    InputProps={{ readOnly: true }}
                    resource={'library'}
                    source={'totalSongs'}
                    label={translate('resources.library.fields.totalSongs')}
                    fullWidth
                    variant="outlined"
                  />
                </Box>
                <Box flex={1} ml="0.5em">
                  <TextInput
                    InputProps={{ readOnly: true }}
                    resource={'library'}
                    source={'totalAlbums'}
                    label={translate('resources.library.fields.totalAlbums')}
                    fullWidth
                    variant="outlined"
                  />
                </Box>
              </Box>

              <Box display="flex">
                <Box flex={1} mr="0.5em">
                  <TextInput
                    InputProps={{ readOnly: true }}
                    resource={'library'}
                    source={'totalArtists'}
                    label={translate('resources.library.fields.totalArtists')}
                    fullWidth
                    variant="outlined"
                  />
                </Box>
                <Box flex={1} ml="0.5em">
                  <TextInput
                    InputProps={{ readOnly: true }}
                    resource={'library'}
                    source={'totalSize'}
                    label={translate('resources.library.fields.totalSize')}
                    format={(v) => formatBytes(v, 2)}
                    fullWidth
                    variant="outlined"
                  />
                </Box>
              </Box>

              <Box display="flex">
                <Box flex={1} mr="0.5em">
                  <TextInput
                    InputProps={{ readOnly: true }}
                    resource={'library'}
                    source={'totalDuration'}
                    label={translate('resources.library.fields.totalDuration')}
                    format={formatDuration2}
                    fullWidth
                    variant="outlined"
                  />
                </Box>
                <Box flex={1} ml="0.5em">
                  <TextInput
                    InputProps={{ readOnly: true }}
                    resource={'library'}
                    source={'totalMissingFiles'}
                    label={translate(
                      'resources.library.fields.totalMissingFiles',
                    )}
                    fullWidth
                    variant="outlined"
                  />
                </Box>
              </Box>

              {/* Timestamps Section */}
              <Box mb="1em">
                <Typography variant="body2" color="textSecondary" gutterBottom>
                  {translate('resources.library.fields.lastScanAt')}
                </Typography>
                <DateField variant="body1" source="lastScanAt" showTime />
              </Box>

              <Box mb="1em">
                <Typography variant="body2" color="textSecondary" gutterBottom>
                  {translate('resources.library.fields.updatedAt')}
                </Typography>
                <DateField variant="body1" source="updatedAt" showTime />
              </Box>

              <Box mb="2em">
                <Typography variant="body2" color="textSecondary" gutterBottom>
                  {translate('resources.library.fields.createdAt')}
                </Typography>
                <DateField variant="body1" source="createdAt" showTime />
              </Box>
            </Box>
          </Box>
        </Box>
      </SimpleForm>
    </Edit>
  )
}

export default LibraryEdit
