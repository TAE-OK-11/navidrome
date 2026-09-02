import { useEffect, useState } from 'react'
import { useNotify, useTranslate } from 'react-admin'
import { FormControl, FormControlLabel, Switch } from '@mui/material'
import { useDispatch } from 'react-redux'
import { httpClient } from '../dataProvider'
import { LibreFmSessionDialog } from '../dialogs'
import { openLibreFmSessionDialog } from '../actions'

export const LibreFmScrobbleToggle = () => {
  const dispatch = useDispatch()
  const notify = useNotify()
  const translate = useTranslate()
  const [linked, setLinked] = useState<boolean | null>(null)

  useEffect(() => {
    httpClient('/api/librefm/link')
      .then((response) => {
        setLinked(response.json.status === true)
      })
      .catch(() => {
        setLinked(false)
      })
  }, [])

  const toggleScrobble = () => {
    if (linked) {
      httpClient('/api/librefm/link', { method: 'DELETE' })
        .then(() => {
          setLinked(false)
          notify('message.libreFmUnlinkSuccess', { type: 'success' })
        })
        .catch(() =>
          notify('message.libreFmUnlinkFailure', { type: 'warning' }),
        )
    } else {
      dispatch(openLibreFmSessionDialog())
    }
  }

  return (
    <>
      <FormControl>
        <FormControlLabel
          control={
            <Switch
              id={'librefm'}
              color="primary"
              checked={linked === true}
              disabled={linked === null}
              onChange={toggleScrobble}
            />
          }
          label={
            <span>{translate('menu.personal.options.libreFmScrobbling')}</span>
          }
        />
      </FormControl>
      <LibreFmSessionDialog setLinked={setLinked} />
    </>
  )
}
