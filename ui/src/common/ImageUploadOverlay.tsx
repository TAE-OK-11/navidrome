// @ts-nocheck -- legacy JavaScript migration; remove after typing this module
import { Box, IconButton, Tooltip } from '@mui/material'
import PhotoCameraIcon from '@mui/icons-material/PhotoCamera'
import DeleteIcon from '@mui/icons-material/Delete'
import { useTranslate, useNotify, useRefresh } from 'react-admin'
import { useCallback, useRef } from 'react'
import config from '../config'
import { REST_URL } from '../consts'
import { httpClient } from '../dataProvider'

const coverOverlaySx = {
  position: 'absolute',
  bottom: 0,
  right: 0,
  display: 'flex',
  gap: '2px',
  p: '2px',
  backgroundColor: 'rgba(0,0,0,0.5)',
  borderRadius: '4px 0 0 0',
  opacity: 0,
  transition: 'opacity 0.2s ease-in-out',
  '*:hover > &': {
    opacity: 1,
  },
} as const

const overlayButtonSx = {
  color: '#fff',
  p: '4px',
  '&:hover': {
    backgroundColor: 'rgba(255,255,255,0.2)',
  },
} as const

const overlayIconSx = { fontSize: '1.2rem' } as const

export const ImageUploadOverlay = ({
  entityType,
  entityId,
  hasUploadedImage,
  onImageChange,
}) => {
  const translate = useTranslate()
  const notify = useNotify()
  const refresh = useRefresh()
  const fileInputRef = useRef(null)

  const canEdit =
    config.enableArtworkUpload || localStorage.getItem('role') === 'admin'

  const handleUploadClick = useCallback((e) => {
    e.stopPropagation()
    if (fileInputRef.current) {
      fileInputRef.current.click()
    }
  }, [])

  const handleFileChange = useCallback(
    async (e) => {
      const file = e.target.files[0]
      if (!file || !entityId) return

      const formData = new FormData()
      formData.append('image', file)

      try {
        await httpClient(`${REST_URL}/${entityType}/${entityId}/image`, {
          method: 'POST',
          headers: new Headers({}),
          body: formData,
        })
        notify(`message.coverUploaded`, { type: 'success' })
        if (onImageChange) onImageChange()
        refresh()
      } catch (err) {
        notify(`message.coverUploadError`, { type: 'warning' })
      }

      e.target.value = ''
    },
    [entityType, entityId, notify, refresh, onImageChange],
  )

  const handleRemoveCover = useCallback(
    async (e) => {
      e.stopPropagation()
      if (!entityId) return

      try {
        await httpClient(`${REST_URL}/${entityType}/${entityId}/image`, {
          method: 'DELETE',
        })
        notify(`message.coverRemoved`, { type: 'success' })
        if (onImageChange) onImageChange()
        refresh()
      } catch (err) {
        notify(`message.coverRemoveError`, { type: 'warning' })
      }
    },
    [entityType, entityId, notify, refresh, onImageChange],
  )

  if (!canEdit) return null

  return (
    <Box sx={coverOverlaySx}>
      <Tooltip title={translate(`message.uploadCover`)}>
        <IconButton
          sx={overlayButtonSx}
          onClick={handleUploadClick}
          size="small"
        >
          <PhotoCameraIcon sx={overlayIconSx} />
        </IconButton>
      </Tooltip>
      {hasUploadedImage && (
        <Tooltip title={translate(`message.removeCover`)}>
          <IconButton
            sx={overlayButtonSx}
            onClick={handleRemoveCover}
            size="small"
          >
            <DeleteIcon sx={overlayIconSx} />
          </IconButton>
        </Tooltip>
      )}
      <input
        ref={fileInputRef}
        type="file"
        accept="image/*"
        style={{ display: 'none' }}
        onChange={handleFileChange}
      />
    </Box>
  )
}
