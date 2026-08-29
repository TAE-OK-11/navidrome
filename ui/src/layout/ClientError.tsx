import React from 'react'
import { Box, Button, Typography } from '@mui/material'
import type { ErrorProps } from 'ra-ui-materialui'

const clearClientState = async () => {
  localStorage.clear()
  sessionStorage.clear()
  if ('caches' in window) {
    const cacheNames = await caches.keys()
    await Promise.all(cacheNames.map((name) => caches.delete(name)))
  }
  if ('serviceWorker' in navigator) {
    const registrations = await navigator.serviceWorker.getRegistrations()
    await Promise.all(
      registrations.map((registration) => registration.unregister()),
    )
  }
  window.location.assign('./')
}

const ClientError = ({
  error,
  resetErrorBoundary,
}: ErrorProps & { resetErrorBoundary?: () => void }) => {
  const message = error instanceof Error ? error.message : String(error || '')
  const retry = () => {
    if (resetErrorBoundary) {
      resetErrorBoundary()
    } else {
      window.location.reload()
    }
  }

  return (
    <Box sx={{ maxWidth: 720, mx: 'auto', p: 3 }}>
      <Typography variant="h4" component="h1" gutterBottom>
        Something went wrong
      </Typography>
      <Typography
        sx={{
          color: 'text.secondary',
          mb: 2,
        }}
      >
        The web client could not finish loading. Try again, or reset the saved
        browser state and sign in again.
      </Typography>
      {message && (
        <Box
          component="pre"
          sx={{ overflow: 'auto', p: 2, bgcolor: 'action.hover' }}
          data-testid="client-error-message"
        >
          {message}
        </Box>
      )}
      <Box sx={{ display: 'flex', gap: 1, mt: 2 }}>
        <Button variant="contained" onClick={retry}>
          Try again
        </Button>
        <Button variant="outlined" onClick={clearClientState}>
          Reset app and sign in again
        </Button>
      </Box>
    </Box>
  )
}

export default ClientError
