import React from 'react'
import { useDispatch, useSelector } from 'react-redux'
import { RecordContextProvider, useTranslate } from 'react-admin'
import {
  Button,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
} from '@mui/material'
import { closeExtendedInfoDialog } from '../actions'
import type { NavidromeRootState } from '../types/redux'

const ExpandInfoDialog = ({
  title,
  content,
}: {
  title?: string
  content: React.ReactNode
}) => {
  const { open, record } = useSelector(
    (state: NavidromeRootState) => state.expandInfoDialog,
  )
  const dispatch = useDispatch()
  const translate = useTranslate()

  const handleClose = (e) => {
    dispatch(closeExtendedInfoDialog())
    e.stopPropagation()
  }

  return (
    <Dialog
      open={open}
      onClose={handleClose}
      aria-labelledby="info-dialog-album"
      fullWidth={true}
      maxWidth={'md'}
    >
      <DialogTitle id="info-dialog-album">
        {translate(title || 'resources.song.actions.info')}
      </DialogTitle>
      <DialogContent>
        {record && (
          <RecordContextProvider value={record as Record<string, unknown>}>
            {content}
          </RecordContextProvider>
        )}
      </DialogContent>
      <DialogActions>
        <Button onClick={handleClose} color="primary">
          {translate('ra.action.close')}
        </Button>
      </DialogActions>
    </Dialog>
  )
}

export default ExpandInfoDialog
