import React from 'react'
import { useRecordContext, useTranslate } from 'react-admin'
import { IconButton, Tooltip, Link } from '@mui/material'
import { ImLastfm2 } from 'react-icons/im'
import MusicBrainz from '../icons/MusicBrainz'
import { intersperse } from '../utils'
import config from '../config'

type AlbumExternalRecord = {
  id: string | number
  albumArtist: string
  name: string
  mbzAlbumId?: string
}

type AlbumExternalLinksProps = {
  className?: string
  record?: AlbumExternalRecord
}

const AlbumExternalLinks = (props: AlbumExternalLinksProps) => {
  const { className } = props
  const translate = useTranslate()
  const record = useRecordContext<AlbumExternalRecord>(props)
  const links: React.ReactNode[] = []

  if (!record) return null

  const addLink = (url: string, title: string, icon: React.ReactNode) => {
    const translatedTitle = translate(title)
    const link = (
      <Link href={url} target="_blank" rel="noopener noreferrer">
        <Tooltip title={translatedTitle}>
          <IconButton size={'small'} aria-label={translatedTitle}>
            {icon}
          </IconButton>
        </Tooltip>
      </Link>
    )
    const id = links.length
    links.push(<span key={`link-${record.id}-${id}`}>{link}</span>)
  }

  if (config.lastFMEnabled) {
    addLink(
      `https://last.fm/music/${
        encodeURIComponent(record.albumArtist) +
        '/' +
        encodeURIComponent(record.name)
      }`,
      'message.openIn.lastfm',
      <ImLastfm2 className="lastfm-icon" />,
    )
  }

  record.mbzAlbumId &&
    addLink(
      `https://musicbrainz.org/release/${record.mbzAlbumId}`,
      'message.openIn.musicbrainz',
      <MusicBrainz className="musicbrainz-icon" />,
    )

  return <div className={className}>{intersperse(links, ' ')}</div>
}

export default AlbumExternalLinks
