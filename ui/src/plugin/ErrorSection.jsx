import React from 'react'
import { Typography } from '@mui/material'
import Alert from '@mui/material/Alert'

export const ErrorSection = ({ error, translate }) => {
  if (!error) return null

  return (
    <Alert severity="error" style={{ marginBottom: 16 }}>
      <Typography variant="subtitle2">
        {translate('resources.plugin.fields.lastError')}
      </Typography>
      <Typography variant="body2">{error}</Typography>
    </Alert>
  )
}
