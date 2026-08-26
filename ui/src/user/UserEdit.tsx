// @ts-nocheck -- legacy JavaScript migration; remove after typing this module
import React, { useCallback } from 'react'
import {
  TextInput,
  BooleanInput,
  DateField,
  PasswordInput,
  Edit,
  required,
  email,
  SimpleForm,
  useTranslate,
  Toolbar,
  SaveButton,
  useDataProvider,
  useNotify,
  useRedirect,
  useRefresh,
  FormDataConsumer,
  usePermissions,
  useRecordContext,
} from 'react-admin'
import { Typography } from '@mui/material'
import { Title } from '../common'
import DeleteUserButton from './DeleteUserButton'
import { LibrarySelectionField } from './LibrarySelectionField'
import { validateUserForm } from './userValidation'

const UserTitle = ({ record: recordOverride }) => {
  const record = useRecordContext({ record: recordOverride })
  const translate = useTranslate()
  const resourceName = translate('resources.user.name', { smart_count: 1 })
  return <Title subTitle={`${resourceName} ${record ? record.name : ''}`} />
}

const UserToolbar = ({ showDelete, ...props }) => (
  <Toolbar {...props} sx={{ display: 'flex', justifyContent: 'space-between' }}>
    <SaveButton disabled={props.pristine} />
    {showDelete && <DeleteUserButton />}
  </Toolbar>
)

const CurrentPasswordInput = ({ formData, isMyself, ...rest }) => {
  const { permissions } = usePermissions()
  return formData.changePassword && (isMyself || permissions !== 'admin') ? (
    <PasswordInput className="ra-input" source="currentPassword" {...rest} />
  ) : null
}

const NewPasswordInput = ({ formData, ...rest }) => {
  const translate = useTranslate()
  return formData.changePassword ? (
    <PasswordInput
      source="password"
      className="ra-input"
      label={translate('resources.user.fields.newPassword')}
      {...rest}
    />
  ) : null
}

const UserEdit = (props) => {
  const { permissions } = props
  const translate = useTranslate()
  const dataProvider = useDataProvider()
  const notify = useNotify()
  const redirect = useRedirect()
  const refresh = useRefresh()

  const isMyself = props.id === localStorage.getItem('userId')
  const getNameHelperText = () =>
    isMyself && {
      helperText: translate('resources.user.helperTexts.name'),
    }
  const canDelete = permissions === 'admin' && !isMyself

  const save = useCallback(
    async (values) => {
      try {
        await dataProvider.update('user', {
          id: values.id,
          data: values,
          previousData: values,
        })
        notify('resources.user.notifications.updated', {
          type: 'info',
          messageArgs: { smart_count: 1 },
        })
        permissions === 'admin' ? redirect('/user') : refresh()
      } catch (error) {
        if (error?.body?.errors) {
          return error.body.errors
        }
        notify('ra.page.error', { type: 'warning' })
      }
    },
    [dataProvider, notify, permissions, redirect, refresh],
  )

  // Custom validation function
  const validateForm = (values) => {
    return validateUserForm(values, translate)
  }

  return (
    <Edit title={<UserTitle />} mutationMode="pessimistic" {...props}>
      <SimpleForm
        toolbar={<UserToolbar showDelete={canDelete} />}
        save={save}
        validate={validateForm}
      >
        {permissions === 'admin' && (
          <TextInput
            spellCheck={false}
            source="userName"
            validate={[required()]}
          />
        )}
        <TextInput
          source="name"
          validate={[required()]}
          {...getNameHelperText()}
        />
        <TextInput spellCheck={false} source="email" validate={[email()]} />
        <BooleanInput source="changePassword" />
        <FormDataConsumer>
          {(formDataProps) => (
            <CurrentPasswordInput
              spellCheck={false}
              isMyself={isMyself}
              {...formDataProps}
            />
          )}
        </FormDataConsumer>
        <FormDataConsumer>
          {(formDataProps) => (
            <NewPasswordInput spellCheck={false} {...formDataProps} />
          )}
        </FormDataConsumer>

        {permissions === 'admin' && (
          <BooleanInput source="isAdmin" defaultValue={false} />
        )}

        {/* Conditional Library Selection for Admin Users Only */}
        {permissions === 'admin' && (
          <FormDataConsumer>
            {({ formData }) => (
              <>
                {!formData.isAdmin && <LibrarySelectionField />}

                {formData.isAdmin && (
                  <Typography
                    variant="body2"
                    color="textSecondary"
                    sx={{ my: 2 }}
                  >
                    {translate('resources.user.message.adminAutoLibraries')}
                  </Typography>
                )}
              </>
            )}
          </FormDataConsumer>
        )}

        <DateField variant="body1" source="lastLoginAt" showTime />
        <DateField variant="body1" source="lastAccessAt" showTime />
        <DateField variant="body1" source="updatedAt" showTime />
        <DateField variant="body1" source="createdAt" showTime />
      </SimpleForm>
    </Edit>
  )
}

export default UserEdit
