// @ts-nocheck -- legacy JavaScript migration; remove after typing this module
import MuiDialogTitle from '@mui/material/DialogTitle'
import Typography from '@mui/material/Typography'
import IconButton from '@mui/material/IconButton'
import CloseIcon from '@mui/icons-material/Close'
import React from 'react'
import { styled } from '@mui/material/styles'

const StyledDialogTitle = styled(MuiDialogTitle)(({ theme }) => ({
  margin: 0,
  padding: theme.spacing(2),
}))

const CloseButton = styled(IconButton)(({ theme }) => ({
  position: 'absolute',
  right: theme.spacing(1),
  top: theme.spacing(1),
  color: theme.palette.grey[500],
}))

export const DialogTitle = ({ children, onClose, ...other }) => {
  return (
    <StyledDialogTitle component="div" {...other}>
      <Typography variant="h5" component="span">
        {children}
      </Typography>
      <CloseButton aria-label="close" onClick={onClose} size="large">
        <CloseIcon />
      </CloseButton>
    </StyledDialogTitle>
  )
}
