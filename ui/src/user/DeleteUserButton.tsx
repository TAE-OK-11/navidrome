// @ts-nocheck -- legacy JavaScript migration; remove after typing this module
import React from 'react'
import DeleteIcon from '@mui/icons-material/Delete'
import { alpha } from '@mui/material/styles'
import clsx from 'clsx'
import {
  useDeleteWithConfirmController,
  Button,
  Confirm,
  useNotify,
  useRedirect,
} from 'react-admin'
import { componentStyleOverride } from '../themes/componentStyleOverride'

const deleteButtonSx = (theme) => ({
  color: theme.palette.error.main,
  '&:hover': {
    backgroundColor: alpha(theme.palette.error.main, 0.12),
    '@media (hover: none)': {
      backgroundColor: 'transparent',
    },
  },
})

const DeleteUserButton = (props) => {
  const { resource, record, basePath, className, onClick, ...rest } = props

  const notify = useNotify()
  const redirect = useRedirect()

  const onSuccess = () => {
    notify('resources.user.notifications.deleted')
    redirect('/user')
  }

  const { open, loading, handleDialogOpen, handleDialogClose, handleDelete } =
    useDeleteWithConfirmController({
      resource,
      record,
      basePath,
      onClick,
      onSuccess,
    })

  return (
    <>
      <Button
        onClick={handleDialogOpen}
        label="ra.action.delete"
        className={clsx('ra-delete-button', className)}
        sx={[
          deleteButtonSx,
          (theme) =>
            componentStyleOverride(
              theme,
              'RaDeleteWithConfirmButton',
              'deleteButton',
            ),
        ]}
        key="button"
        {...rest}
      >
        <DeleteIcon />
      </Button>
      <Confirm
        isOpen={open}
        loading={loading}
        title="message.delete_user_title"
        content="message.delete_user_content"
        translateOptions={{
          name: record.name,
        }}
        onConfirm={handleDelete}
        onClose={handleDialogClose}
      />
    </>
  )
}

export default DeleteUserButton
