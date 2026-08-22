import React, { useCallback } from 'react'
import {
  BooleanInput,
  Create,
  email,
  FormDataConsumer,
  PasswordInput,
  required,
  SimpleForm,
  TextInput,
  useDataProvider,
  useNotify,
  useRedirect,
  useTranslate,
} from 'react-admin'
import { Typography } from '@mui/material'
import { Title } from '../common'
import { LibrarySelectionField } from './LibrarySelectionField.jsx'

const UserCreate = (props) => {
  const translate = useTranslate()
  const dataProvider = useDataProvider()
  const notify = useNotify()
  const redirect = useRedirect()
  const resourceName = translate('resources.user.name', { smart_count: 1 })
  const title = translate('ra.page.create', {
    name: `${resourceName}`,
  })

  const save = useCallback(
    async (values) => {
      try {
        await dataProvider.create('user', { data: values })
        notify('resources.user.notifications.created', 'info', {
          smart_count: 1,
        })
        redirect('/user')
      } catch (error) {
        if (error.body.errors) {
          return error.body.errors
        }
      }
    },
    [dataProvider, notify, redirect],
  )

  // Custom validation function
  const validateUserForm = (values) => {
    const errors = {}
    // Library selection is optional for non-admin users since they will be auto-assigned to default libraries
    // No validation required for library selection
    return errors
  }

  return (
    <Create title={<Title subTitle={title} />} {...props}>
      <SimpleForm save={save} validate={validateUserForm} variant={'outlined'}>
        <TextInput
          spellCheck={false}
          source="userName"
          validate={[required()]}
        />
        <TextInput source="name" validate={[required()]} />
        <TextInput spellCheck={false} source="email" validate={[email()]} />
        <PasswordInput
          spellCheck={false}
          source="password"
          validate={[required()]}
        />
        <BooleanInput source="isAdmin" defaultValue={false} />

        {/* Conditional Library Selection */}
        <FormDataConsumer>
          {({ formData }) => (
            <>
              {!formData.isAdmin && <LibrarySelectionField />}

              {formData.isAdmin && (
                <Typography
                  variant="body2"
                  color="textSecondary"
                  style={{ marginTop: 16, marginBottom: 16 }}
                >
                  {translate('resources.user.message.adminAutoLibraries')}
                </Typography>
              )}
            </>
          )}
        </FormDataConsumer>
      </SimpleForm>
    </Create>
  )
}

export default UserCreate
