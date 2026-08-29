import { SimpleForm, useTranslate } from 'react-admin'
import { useDispatch, useSelector } from 'react-redux'
import {
  Button,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
} from '@mui/material'
import subsonic from '../subsonic'
import { closeDownloadMenu, DOWNLOAD_MENU_ARTIST } from '../actions'
import { formatBytes } from '../utils'
import { artistDownloadSize } from '../common/artist'
import { useTranscodingOptions } from './useTranscodingOptions'
import type { NavidromeRootState, DownloadMenuDialogState } from '../types/redux'

type DownloadRecord = NonNullable<DownloadMenuDialogState['record']>

const DownloadMenuDialog = () => {
  const { open, record, recordType } = useSelector(
    (state: NavidromeRootState) => state.downloadMenuDialog,
  )
  const downloadRecord = record as DownloadRecord | undefined
  const dispatch = useDispatch()
  const translate = useTranslate()

  const { TranscodingOptionsInput, format, maxBitRate, originalFormat } =
    useTranscodingOptions()

  // Artist downloads only include album-artist songs, so show that size
  const downloadSize =
    recordType === DOWNLOAD_MENU_ARTIST
      ? artistDownloadSize(downloadRecord)
      : downloadRecord?.size

  const handleClose = (e) => {
    dispatch(closeDownloadMenu())
    e.stopPropagation()
  }

  const handleDownload = (e) => {
    if (downloadRecord) {
      const id = downloadRecord.mediaFileId || downloadRecord.id
      if (originalFormat) {
        subsonic.download(String(id), 'raw')
      } else {
        subsonic.download(String(id), format, maxBitRate?.toString())
      }
      dispatch(closeDownloadMenu())
    }
    e.stopPropagation()
  }

  return (
    <Dialog
      open={open}
      onClose={handleClose}
      aria-labelledby="download-dialog"
      fullWidth={true}
      maxWidth={'sm'}
    >
      <DialogTitle id="download-dialog">
        {recordType &&
          translate('message.downloadDialogTitle', {
            resource: translate(`resources.${recordType}.name`, {
              smart_count: 1,
            }).toLocaleLowerCase(),
            name: downloadRecord?.name || downloadRecord?.title,
            size: formatBytes(downloadSize),
          })}
      </DialogTitle>
      <DialogContent>
        <SimpleForm toolbar={null}>
          <TranscodingOptionsInput
            basePath=""
            fullWidth
            label={translate('message.downloadOriginalFormat')}
          />
        </SimpleForm>
      </DialogContent>
      <DialogActions>
        <Button onClick={handleClose} color="secondary">
          {translate('ra.action.close')}
        </Button>
        <Button onClick={handleDownload} color="primary">
          {translate('ra.action.download')}
        </Button>
      </DialogActions>
    </Dialog>
  )
}

export default DownloadMenuDialog
