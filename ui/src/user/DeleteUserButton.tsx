import React from 'react'
import type { ReactEventHandler } from 'react'
import DeleteIcon from '@mui/icons-material/Delete'
import { alpha, type Theme } from '@mui/material/styles'
import clsx from 'clsx'
import {
  type RaRecord,
  useDeleteWithConfirmController,
  Button,
  Confirm,
  useNotify,
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

type UserRecord = RaRecord & { name?: string }

type DeleteUserButtonProps = {
  resource?: string
  record?: UserRecord
  className?: string
  onClick?: ReactEventHandler
}

const DeleteUserButton = ({
  resource,
  record,
  className,
  onClick,
}: DeleteUserButtonProps) => {
  const notify = useNotify()
  const redirect = useRedirect()

  const onSuccess = () => {
    notify('resources.user.notifications.deleted')
    redirect('/user')
  }

  const {
    open,
    isPending,
    handleDialogOpen,
    handleDialogClose,
    handleDelete,
  } = useDeleteWithConfirmController({
    resource,
    record,
    onClick,
    mutationOptions: { onSuccess },
  })

  return (
    <>
      <Button
        onClick={handleDialogOpen}
        label="ra.action.delete"
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
        key="button"
      >
        <DeleteIcon />
      </Button>
      <Confirm
        isOpen={open}
        loading={isPending}
        title="message.delete_user_title"
        content="message.delete_user_content"
        titleTranslateOptions={{ name: record?.name ?? '' }}
        contentTranslateOptions={{ name: record?.name ?? '' }}
        onConfirm={handleDelete}
        onClose={handleDialogClose}
      />
    </>
  )
}

export default DeleteUserButton
