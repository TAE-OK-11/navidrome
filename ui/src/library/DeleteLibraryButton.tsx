import React from 'react'
import DeleteIcon from '@mui/icons-material/Delete'
import { alpha, type Theme } from '@mui/material/styles'
import clsx from 'clsx'
import {
  type RaRecord,
  useNotify,
  useDeleteWithConfirmController,
  Button,
  Confirm,
  useTranslate,
  useRedirect,
} from 'react-admin'
import { componentStyleOverride } from '../themes/componentStyleOverride'

const deleteButtonSx = (theme: Theme) => ({
  color: theme.palette.error.main,
  '&:hover': {
    backgroundColor: alpha(theme.palette.error.main, 0.12),
    '@media (hover: none)': {
      backgroundColor: 'transparent',
    },
  },
})

type DeleteLibraryButtonProps = {
  record?: RaRecord
  resource?: string
  className?: string
}

const DeleteLibraryButton = ({
  record,
  resource,
  className,
}: DeleteLibraryButtonProps) => {
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

  const { open, isPending, handleDialogOpen, handleDialogClose, handleDelete } =
    useDeleteWithConfirmController({
      resource,
      record,
      mutationOptions: { onSuccess },
    })

  return (
    <>
      <Button
        label="ra.action.delete"
        onClick={handleDialogOpen}
        disabled={isPending}
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
      >
        <DeleteIcon />
      </Button>
      <Confirm
        isOpen={open}
        loading={isPending}
        title={translate('resources.library.name', { smart_count: 1 })}
        content={translate('resources.library.messages.deleteConfirm')}
        onConfirm={handleDelete}
        onClose={handleDialogClose}
      />
    </>
  )
}

export default DeleteLibraryButton
