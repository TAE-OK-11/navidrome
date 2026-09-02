import {
  Alert,
  Box,
  Button,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  IconButton,
  List,
  ListItem,
  ListItemText,
  TextField,
  Typography,
} from '@mui/material'
import DeleteIcon from '@mui/icons-material/Delete'
import { useCallback, useEffect, useState } from 'react'
import { useNotify, useTranslate } from 'react-admin'
import { httpClient } from '../dataProvider'

type APIKey = {
  id: string
  name: string
  createdAt?: string
  lastUsedAt?: string
  expiresAt?: string
}

type CreateAPIKeyResponse = {
  key: APIKey
  token: string
}

export const ApiKeysManager = () => {
  const translate = useTranslate()
  const notify = useNotify()
  const [keys, setKeys] = useState<APIKey[]>([])
  const [name, setName] = useState('')
  const [expiresAt, setExpiresAt] = useState('')
  const [loading, setLoading] = useState(false)
  const [createdToken, setCreatedToken] = useState<string | null>(null)

  const formatExpiry = (keyExpiresAt?: string) => {
    if (!keyExpiresAt) {
      return translate('menu.personal.options.apiKeyNoExpiry')
    }
    const expiry = new Date(keyExpiresAt)
    if (Number.isNaN(expiry.getTime())) {
      return null
    }
    if (expiry.getTime() <= Date.now()) {
      return translate('menu.personal.options.apiKeyExpired')
    }
    return translate('menu.personal.options.apiKeyExpires', {
      date: expiry.toLocaleString(),
    })
  }

  const loadKeys = useCallback(() => {
    setLoading(true)
    httpClient('/api/apikey')
      .then((response) => {
        setKeys(response.json || [])
      })
      .catch(() => {
        notify('ra.message.error', { type: 'warning' })
      })
      .finally(() => setLoading(false))
  }, [notify])

  useEffect(() => {
    loadKeys()
  }, [loadKeys])

  const createKey = () => {
    const trimmed = name.trim()
    if (!trimmed) {
      return
    }
    setLoading(true)
    const payload: { name: string; expiresAt?: string } = { name: trimmed }
    if (expiresAt) {
      payload.expiresAt = new Date(expiresAt).toISOString()
    }
    httpClient('/api/apikey', {
      method: 'POST',
      body: JSON.stringify(payload),
    })
      .then((response) => {
        const body = response.json as CreateAPIKeyResponse
        setCreatedToken(body.token)
        setName('')
        setExpiresAt('')
        loadKeys()
      })
      .catch(() => {
        notify('ra.message.error', { type: 'warning' })
      })
      .finally(() => setLoading(false))
  }

  const deleteKey = (id: string) => {
    setLoading(true)
    httpClient(`/api/apikey/${id}`, { method: 'DELETE' })
      .then(() => loadKeys())
      .catch(() => notify('ra.message.error', { type: 'warning' }))
      .finally(() => setLoading(false))
  }

  return (
    <Box sx={{ width: '100%', mt: 2 }}>
      <Typography variant="subtitle1" sx={{ mb: 1 }}>
        {translate('menu.personal.options.apiKeys')}
      </Typography>
      <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
        {translate('menu.personal.options.apiKeysHelp')}
      </Typography>
      <Box sx={{ display: 'flex', gap: 1, mb: 2, flexWrap: 'wrap' }}>
        <TextField
          size="small"
          fullWidth
          label={translate('menu.personal.options.apiKeyName')}
          value={name}
          onChange={(event) => setName(event.target.value)}
          disabled={loading}
          sx={{ flex: '1 1 220px' }}
        />
        <TextField
          size="small"
          type="datetime-local"
          label={translate('menu.personal.options.apiKeyExpiresAt')}
          value={expiresAt}
          onChange={(event) => setExpiresAt(event.target.value)}
          disabled={loading}
          slotProps={{ inputLabel: { shrink: true } }}
          sx={{ flex: '0 1 240px' }}
        />
        <Button
          variant="contained"
          onClick={createKey}
          disabled={loading || !name.trim()}
          sx={{ alignSelf: 'center' }}
        >
          {translate('menu.personal.options.apiKeyCreate')}
        </Button>
      </Box>
      <List dense>
        {keys.map((key) => (
          <ListItem
            key={key.id}
            secondaryAction={
              <IconButton
                edge="end"
                onClick={() => deleteKey(key.id)}
                disabled={loading}
              >
                <DeleteIcon />
              </IconButton>
            }
          >
            <ListItemText
              primary={key.name}
              secondary={[
                key.lastUsedAt
                  ? translate('menu.personal.options.apiKeyLastUsed', {
                      date: new Date(key.lastUsedAt).toLocaleString(),
                    })
                  : translate('menu.personal.options.apiKeyNeverUsed'),
                key.createdAt
                  ? translate('menu.personal.options.apiKeyCreated', {
                      date: new Date(key.createdAt).toLocaleString(),
                    })
                  : null,
                formatExpiry(key.expiresAt),
              ]
                .filter(Boolean)
                .join(' · ')}
            />
          </ListItem>
        ))}
      </List>
      <Dialog
        open={createdToken != null}
        onClose={() => setCreatedToken(null)}
        maxWidth="sm"
        fullWidth
      >
        <DialogTitle>
          {translate('menu.personal.options.apiKeyCreatedTitle')}
        </DialogTitle>
        <DialogContent>
          <Alert severity="warning" sx={{ mb: 2 }}>
            {translate('menu.personal.options.apiKeyCreatedWarning')}
          </Alert>
          <TextField
            fullWidth
            multiline
            minRows={3}
            value={createdToken || ''}
            slotProps={{ htmlInput: { readOnly: true } }}
          />
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setCreatedToken(null)}>
            {translate('ra.action.close')}
          </Button>
        </DialogActions>
      </Dialog>
    </Box>
  )
}
