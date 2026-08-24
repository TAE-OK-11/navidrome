// @ts-nocheck -- legacy JavaScript migration; remove after typing this module
import React from 'react'
import DeleteIcon from '@mui/icons-material/Delete'
import { alpha } from '@mui/material/styles'
import clsx from 'clsx'
import {
  useNotify,
  useDeleteWithConfirmController,
  Button,
  Confirm,
  useTranslate,
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

const DeleteLibraryButton = ({
  record,
  resource,
  basePath,
  className,
  ...props
}) => {
  const translate = useTranslate()
  const notify = useNotify()
  const redirect = useRedirect()

  const onSuccess = () => {
    notify('resources.library.notifications.deleted', {
      type: 'info',
      messageArgs: { smart_count: 1 },
    })
    redirect('/library')
  }

  const { open, loading, handleDialogOpen, handleDialogClose, handleDelete } =
    useDeleteWithConfirmController({
      resource,
      record,
      basePath,
      onSuccess,
    })

  return (
    <>
      <Button
        label="ra.action.delete"
        onClick={handleDialogOpen}
        disabled={loading}
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
        {...props}
      >
        <DeleteIcon />
      </Button>
      <Confirm
        isOpen={open}
        loading={loading}
        title={translate('resources.library.name', { smart_count: 1 })}
        content={translate('resources.library.messages.deleteConfirm')}
        onConfirm={handleDelete}
        onClose={handleDialogClose}
      />
    </>
  )
}

export default DeleteLibraryButton
