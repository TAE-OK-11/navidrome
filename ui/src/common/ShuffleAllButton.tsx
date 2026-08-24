import React from 'react'
import { Button, useDataProvider, useNotify, useTranslate } from 'react-admin'
import { useDispatch } from 'react-redux'
import ShuffleIcon from '@mui/icons-material/Shuffle'
import { playTracks } from '../actions'
import PropTypes from 'prop-types'

type ShuffleAllButtonProps = {
  filters?: Record<string, unknown>
}

export const ShuffleAllButton = ({ filters = {} }: ShuffleAllButtonProps) => {
  const translate = useTranslate()
  const dataProvider = useDataProvider()
  const dispatch = useDispatch()
  const notify = useNotify()
  filters = { ...filters, missing: false }

  const handleOnClick = () => {
    dataProvider
      .getList('song', {
        pagination: { page: 1, perPage: 500 },
        sort: { field: 'random', order: 'ASC' },
        filter: filters,
      })
      .then((res) => {
        const data: Record<string, (typeof res.data)[number]> = {}
        res.data.forEach((song) => {
          data[String(song.id)] = song
        })
        dispatch(playTracks(data))
      })
      .catch(() => {
        notify('ra.page.error', { type: 'warning' })
      })
  }

  return (
    <Button
      onClick={handleOnClick}
      label={translate('resources.song.actions.shuffleAll')}
    >
      <ShuffleIcon />
    </Button>
  )
}

ShuffleAllButton.propTypes = {
  filters: PropTypes.object,
}
