import React from 'react'
import { Typography } from '@mui/material'
import Alert from '@mui/material/Alert'

type ErrorSectionProps = {
  error?: string | null
  translate: (key: string) => string
}

export const ErrorSection = ({ error, translate }: ErrorSectionProps) => {
  if (!error) return null

  return (
    <Alert severity="error" sx={{ mb: 2 }}>
      <Typography variant="subtitle2">
        {translate('resources.plugin.fields.lastError')}
      </Typography>
      <Typography variant="body2">{error}</Typography>
    </Alert>
  )
}
