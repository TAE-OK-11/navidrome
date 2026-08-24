// @ts-nocheck -- legacy JavaScript migration; remove after typing this module
import { useEffect, useState } from 'react'
import { useNotify, useTranslate } from 'react-admin'
import { FormControl, FormControlLabel, Switch } from '@mui/material'
import { httpClient } from '../dataProvider'
import { ListenBrainzTokenDialog } from '../dialogs'
import { useDispatch } from 'react-redux'
import { openListenBrainzTokenDialog } from '../actions'

export const ListenBrainzScrobbleToggle = () => {
  const dispatch = useDispatch()
  const notify = useNotify()
  const translate = useTranslate()
  const [linked, setLinked] = useState(null)

  const toggleScrobble = () => {
    if (linked) {
      httpClient('/api/listenbrainz/link', { method: 'DELETE' })
        .then(() => {
          setLinked(false)
          notify('message.listenBrainzUnlinkSuccess', { type: 'success' })
        })
        .catch(() =>
          notify('message.listenBrainzUnlinkFailure', { type: 'warning' }),
        )
    } else {
      dispatch(openListenBrainzTokenDialog())
    }
  }

  useEffect(() => {
    httpClient('/api/listenbrainz/link')
      .then((response) => {
        setLinked(response.json.status === true)
      })
      .catch(() => {
        setLinked(false)
      })
  }, [])

  return (
    <>
      <FormControl>
        <FormControlLabel
          control={
            <Switch
              id={'listenbrainz'}
              color="primary"
              checked={linked === true}
              disabled={linked === null}
              onChange={toggleScrobble}
            />
          }
          label={
            <span>
              {translate('menu.personal.options.listenBrainzScrobbling')}
            </span>
          }
        />
      </FormControl>
      <ListenBrainzTokenDialog setLinked={setLinked} />
    </>
  )
}
