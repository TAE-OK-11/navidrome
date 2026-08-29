import React from 'react'
import { useTranslate } from 'react-admin'
import { Box, IconButton, Tooltip, Link } from '@mui/material'

import { ImLastfm2 } from 'react-icons/im'
import MusicBrainz from '../icons/MusicBrainz'
import { intersperse, isLastFmURL } from '../utils'
import config from '../config'
import type { ArtistRecord } from '../types/records'

type ArtistInfo = {
  biography?: string
  lastFmUrl?: string
  musicBrainzId?: string
}

type ArtistExternalLinksProps = {
  artistInfo?: ArtistInfo
  record: ArtistRecord
}

const ArtistExternalLinks = ({
  artistInfo,
  record,
}: ArtistExternalLinksProps) => {
  const translate = useTranslate()
  const linkButtons: React.ReactNode[] = []
  const lastFMlink = artistInfo?.biography?.match(
    /<a\s+(?:[^>]*?\s+)?href=(["'])(.*?)\1/,
  )

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
    const id = linkButtons.length
    linkButtons.push(<span key={`link-${record.id}-${id}`}>{link}</span>)
  }

  if (config.lastFMEnabled) {
    if (lastFMlink && isLastFmURL(lastFMlink[2])) {
      addLink(
        lastFMlink[2],
        'message.openIn.lastfm',
        <ImLastfm2 className="lastfm-icon" />,
      )
    } else if (artistInfo?.lastFmUrl && isLastFmURL(artistInfo.lastFmUrl)) {
      addLink(
        artistInfo.lastFmUrl,
        'message.openIn.lastfm',
        <ImLastfm2 className="lastfm-icon" />,
      )
    }
  }

  artistInfo?.musicBrainzId &&
    addLink(
      `https://musicbrainz.org/artist/${artistInfo.musicBrainzId}`,
      'message.openIn.musicbrainz',
      <MusicBrainz className="musicbrainz-icon" />,
    )

  return (
    <Box sx={{ minHeight: '1.875em' }}>{intersperse(linkButtons, ' ')}</Box>
  )
}

export default ArtistExternalLinks
